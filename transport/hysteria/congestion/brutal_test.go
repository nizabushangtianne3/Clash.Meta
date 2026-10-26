package congestion

import (
	"testing"
	"time"

	"github.com/metacubex/quic-go/congestion"
	"github.com/metacubex/quic-go/monotime"
	"github.com/stretchr/testify/assert"
)

type stubRTT struct {
	latest   time.Duration
	smoothed time.Duration
}

func (s stubRTT) MinRTT() time.Duration                  { return 0 }
func (s stubRTT) LatestRTT() time.Duration               { return s.latest }
func (s stubRTT) SmoothedRTT() time.Duration             { return s.smoothed }
func (s stubRTT) MeanDeviation() time.Duration           { return 0 }
func (s stubRTT) MaxAckDelay() time.Duration             { return 0 }
func (s stubRTT) PTO(bool) time.Duration                 { return 0 }
func (s stubRTT) UpdateRTT(time.Duration, time.Duration) {}
func (s stubRTT) SetMaxAckDelay(time.Duration)           {}
func (s stubRTT) SetInitialRTT(time.Duration)            {}

func atSeconds(seconds int64) monotime.Time {
	return monotime.Time(time.Duration(seconds) * time.Second)
}

func acked(n int) []congestion.AckedPacketInfo {
	return make([]congestion.AckedPacketInfo, n)
}

func lost(n int) []congestion.LostPacketInfo {
	return make([]congestion.LostPacketInfo, n)
}

// Without an rtt there is no bandwidth delay product to size against, so the
// control falls back to the fixed 10 KiB.
func TestBrutalSenderCongestionWindowWithoutRTT(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)
	sender.SetRTTStatsProvider(stubRTT{})

	assert.EqualValues(t, 10240, sender.GetCongestionWindow())
}

// The window is the bandwidth delay product widened by one and a half, and
// divided by the ack rate so loss opens it further: that is what makes brutal
// hold its rate rather than back off.
func TestBrutalSenderCongestionWindowIsTheWidenedBDP(t *testing.T) {
	const bps = congestion.ByteCount(4 * 1024 * 1024)
	sender := NewBrutalSender(bps)
	sender.SetRTTStatsProvider(stubRTT{latest: 100 * time.Millisecond})

	assert.EqualValues(t, 629145, sender.GetCongestionWindow())

	// A poorer ack rate widens the same window.
	sender.OnCongestionEventEx(0, atSeconds(100), acked(20), lost(80))
	sender.OnCongestionEventEx(0, atSeconds(101), acked(20), lost(80))
	assert.InDelta(t, 0.8, sender.ackRate, 1e-9)
	assert.EqualValues(t, 786432, sender.GetCongestionWindow())
}

// Below the sample floor the control refuses to guess, so the ack rate stays
// at one and the window keeps its configured shape.
func TestBrutalSenderAckRateStaysOneBelowTheSampleFloor(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	for _, second := range []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		sender.OnCongestionEventEx(0, atSeconds(second), acked(4), lost(1))
	}

	assert.InDelta(t, 1.0, sender.ackRate, 1e-9)
}

func TestBrutalSenderAckRateTracksTheObservedAckFraction(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	// Fifty clean packets inside one sampled second.
	sender.OnCongestionEventEx(0, atSeconds(1), acked(50), nil)
	assert.InDelta(t, 1.0, sender.ackRate, 1e-9)

	// A tenth of the packets dropped in the same second.
	sender.OnCongestionEventEx(0, atSeconds(1), acked(45), lost(5))
	assert.InDelta(t, 0.95, sender.ackRate, 1e-9)
}

// Anything below the floor is not believed: the control pins the rate at
// minAckRate so a bad second cannot size the window from noise.
func TestBrutalSenderClampsTheAckRateAtItsFloor(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	sender.OnCongestionEventEx(0, atSeconds(1), acked(5), lost(95))
	assert.InDelta(t, 0.8, sender.ackRate, 1e-9)
}

// A sample is dropped once the slot that would hold it is older than the
// window, which is what keeps a stalled second from pinning the rate.
func TestBrutalSenderDropsSamplesOlderThanTheWindow(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	// Second 1 lands in slot 1 and is the only sample.
	sender.OnCongestionEventEx(0, atSeconds(1), acked(60), nil)
	assert.InDelta(t, 1.0, sender.ackRate, 1e-9)

	// Second 6 reuses slot 1, so the second 1 sample is overwritten rather
	// than added to.
	sender.OnCongestionEventEx(0, atSeconds(6), acked(30), lost(30))
	slot := 6 % pktInfoSlotCount
	assert.EqualValues(t, 30, sender.pktInfoSlots[slot].AckCount)
	assert.EqualValues(t, 30, sender.pktInfoSlots[slot].LossCount)
	// Half the sampled packets were lost, so the floor clamps the rate.
	assert.InDelta(t, 0.8, sender.ackRate, 1e-9)
}

func TestBrutalSenderAccumulatesWithinASecond(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	sender.OnCongestionEventEx(0, atSeconds(2), acked(10), lost(1))
	sender.OnCongestionEventEx(0, atSeconds(2), acked(10), lost(1))

	slot := 2 % pktInfoSlotCount
	assert.EqualValues(t, 20, sender.pktInfoSlots[slot].AckCount)
	assert.EqualValues(t, 2, sender.pktInfoSlots[slot].LossCount)
	assert.EqualValues(t, 2, sender.pktInfoSlots[slot].Timestamp)
}

func TestBrutalSenderSendingIsBudgetedByRate(t *testing.T) {
	sender := NewBrutalSender(congestion.ByteCount(4 * 1024 * 1024))
	sender.SetRTTStatsProvider(stubRTT{latest: 100 * time.Millisecond})

	// With nothing in flight the pacer is free to burst, and a datagram fits.
	assert.True(t, sender.HasPacingBudget(atSeconds(1)))
	assert.True(t, sender.CanSend(0))

	// The window is wide enough for the full burst the pacer allows.
	window := sender.GetCongestionWindow()
	assert.True(t, sender.CanSend(window-1))
	assert.False(t, sender.CanSend(window))

	// Spending the whole burst at one instant leaves nothing for the next
	// datagram.
	for range maxBurstPackets {
		sender.OnPacketSent(atSeconds(1), sender.maxDatagramSize, 1, sender.maxDatagramSize, true)
	}
	assert.False(t, sender.HasPacingBudget(atSeconds(1)))
	assert.EqualValues(t, initMaxDatagramSize, sender.maxDatagramSize)

	// The pacer's burst follows the datagram size the path allows.
	sender.SetMaxDatagramSize(1400)
	assert.EqualValues(t, 1400, sender.maxDatagramSize)
}

func TestBrutalSenderIsNeverInSlowStartOrRecovery(t *testing.T) {
	sender := NewBrutalSender(1024 * 1024)

	assert.False(t, sender.InSlowStart())
	assert.False(t, sender.InRecovery())
	sender.MaybeExitSlowStart()
	sender.OnRetransmissionTimeout(true)
	sender.OnPacketAcked(1, 1200, 1200, atSeconds(1))
	sender.OnCongestionEvent(1, 0, 1200)

	// None of those move the rate: brutal only learns from congestion events.
	assert.InDelta(t, 1.0, sender.ackRate, 1e-9)
}

func TestBrutalSenderTimeUntilSendReportsThePacingDelay(t *testing.T) {
	sender := NewBrutalSender(congestion.ByteCount(4 * 1024 * 1024))

	// With burst budget left the pacer says send now.
	assert.Equal(t, monotime.Time(0), sender.TimeUntilSend(0))

	// After the burst is spent the answer is a moment in the future, which is
	// what quic-go hands to its timer.
	for range maxBurstPackets {
		sender.OnPacketSent(atSeconds(1), sender.maxDatagramSize, 1, sender.maxDatagramSize, true)
	}
	assert.NotEqual(t, monotime.Time(0), sender.TimeUntilSend(0))
}

// A packet larger than the whole burst exhausts the pacer, so the next send
// has to wait for budget rather than borrow it.
func TestBrutalSenderOversizedPacketEmptiesThePacer(t *testing.T) {
	sender := NewBrutalSender(congestion.ByteCount(4 * 1024 * 1024))
	const oversized = 1 << 20

	sender.OnPacketSent(atSeconds(1), oversized, 1, oversized, true)

	assert.False(t, sender.HasPacingBudget(atSeconds(1)))
	assert.NotEqual(t, monotime.Time(0), sender.TimeUntilSend(0))
}
