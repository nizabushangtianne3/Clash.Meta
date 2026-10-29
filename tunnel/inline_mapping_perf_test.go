package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestPacketSenderKeepsOneMappingInline(t *testing.T) {
	sender := newPacketSender().(*packetSender)
	t.Cleanup(sender.Close)

	origin := &C.Metadata{Host: "dns.example"}
	target := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.20")}
	sender.AddMapping(origin, target)

	require.True(t, sender.singleMapped)
	require.Nil(t, sender.originToTarget)
	require.Nil(t, sender.targetToOrigin)
	require.Equal(t, target.DstIP, sender.targetFor(packetOriginKey(origin)))
	require.Equal(t, target.DstIP, sender.RestoreReadFrom(target.DstIP))
}

func TestPacketSenderPromotesOnlyForMultipleOrigins(t *testing.T) {
	sender := newPacketSender().(*packetSender)
	t.Cleanup(sender.Close)

	firstOrigin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}
	secondOrigin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.3")}
	target := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.20")}
	sender.AddMapping(firstOrigin, target)
	sender.AddMapping(secondOrigin, target)

	require.NotNil(t, sender.originToTarget)
	require.NotNil(t, sender.targetToOrigin)
	require.Equal(t, target.DstIP, sender.targetFor(packetOriginKey(firstOrigin)))
	require.Equal(t, target.DstIP, sender.targetFor(packetOriginKey(secondOrigin)))
	require.Equal(t, firstOrigin.DstIP, sender.RestoreReadFrom(target.DstIP))
}

func TestPacketSenderUpgradesAnUnresolvedFirstTarget(t *testing.T) {
	sender := newPacketSender().(*packetSender)
	t.Cleanup(sender.Close)

	origin := &C.Metadata{Host: "dns.example"}
	sender.AddMapping(origin, &C.Metadata{Host: "dns.example"})
	resolved := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.20")}
	sender.AddMapping(origin, resolved)

	require.Nil(t, sender.originToTarget)
	require.Nil(t, sender.targetToOrigin)
	require.Equal(t, resolved.DstIP, sender.targetFor(packetOriginKey(origin)))
	require.Equal(t, resolved.DstIP, sender.RestoreReadFrom(resolved.DstIP))
}
