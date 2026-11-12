package encryption

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The record header is the protocol's framing magic: a wrong byte anywhere in
// it must be rejected, because the length it carries is trusted untrusted.
func TestDecodeHeader(t *testing.T) {
	valid := make([]byte, 5)
	EncodeHeader(valid, 1234)
	require.Equal(t, 1234, mustDecodeHeader(t, valid))
	require.Equal(t, 17, mustDecodeHeader(t, []byte{23, 3, 3, 0, 17}))
	require.Equal(t, 17000, mustDecodeHeader(t, []byte{23, 3, 3, 66, 104}))

	for name, bad := range map[string][]byte{
		"version":             {22, 3, 3, 0, 17},
		"content type":        {23, 2, 3, 0, 17},
		"second content type": {23, 3, 2, 0, 17},
		"too short":           {23, 3, 3, 0, 16},
		"too long":            {23, 3, 3, 66, 233},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeHeader(bad)
			require.ErrorIs(t, err, ErrInvalidHeader)
		})
	}
}

func mustDecodeHeader(t *testing.T, h []byte) int {
	t.Helper()
	l, err := DecodeHeader(h)
	require.NoError(t, err)
	return l
}

func TestEncodeHeaderWritesTheTLSRecordShape(t *testing.T) {
	h := make([]byte, 5)
	EncodeHeader(h, 17000)
	require.Equal(t, byte(23), h[0])
	require.Equal(t, byte(3), h[1])
	require.Equal(t, byte(3), h[2])
	require.Equal(t, byte(66), h[3])
	require.Equal(t, byte(104), h[4])
}

func TestEncodeDecodeLengthRoundTrips(t *testing.T) {
	for _, l := range []int{0, 1, 255, 256, 65535} {
		b := EncodeLength(l)
		require.Len(t, b, 2)
		require.Equal(t, l, DecodeLength(b))
	}
}

func TestIncreaseNonceCarriesAcrossBytes(t *testing.T) {
	nonce := make([]byte, 12)
	IncreaseNonce(nonce)
	require.Equal(t, byte(1), nonce[11])

	last := make([]byte, 12)
	last[11] = 255
	IncreaseNonce(last)
	require.Equal(t, byte(0), last[11])
	require.Equal(t, byte(1), last[10])

	// Wrapping around leaves every byte at zero, which is the one value the
	// caller can detect because the seal rotated.
	full := MaxNonce
	Increased := append([]byte{}, full...)
	IncreaseNonce(Increased)
	require.Equal(t, make([]byte, 12), Increased)
}

// Seal and Open both drive the counter themselves when no explicit nonce is
// given, so a single AEAD can carry a whole session in order.
func TestAEADCarriesItsNonceAcrossCalls(t *testing.T) {
	key := randomKey(t)
	plaintext := []byte("first record")

	sealer := NewAEAD([]byte("ctx"), key, true)
	opener := NewAEAD([]byte("ctx"), key, true)
	sealed := sealer.Seal(nil, nil, plaintext, nil)
	require.NotEmpty(t, sealed)

	second := sealer.Seal(nil, nil, plaintext, nil)
	require.NotEqual(t, sealed, second, "the counter must move between records")

	// The peer starts from the same zero nonce, so the first Open matches the
	// first Seal and a later one does not reuse the earlier counter.
	opened, err := opener.Open(nil, nil, sealed, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, opened)

	_, err = opener.Open(nil, nil, sealed, nil)
	require.Error(t, err)
}

func TestAEADAdditionalDataIsAuthenticated(t *testing.T) {
	key := randomKey(t)
	sealer := NewAEAD([]byte("ctx"), key, true)

	sealed := sealer.Seal(nil, nil, []byte("payload"), []byte("aad"))
	_, err := NewAEAD([]byte("ctx"), key, true).Open(nil, nil, sealed, []byte("other"))
	require.Error(t, err)

	opened, err := NewAEAD([]byte("ctx"), key, true).Open(nil, nil, sealed, []byte("aad"))
	require.NoError(t, err)
	require.Equal(t, "payload", string(opened))
}

func TestAEADContextSeparatesTheStreams(t *testing.T) {
	key := randomKey(t)
	up := NewAEAD([]byte("upload"), key, true)
	down := NewAEAD([]byte("download"), key, true)

	sealed := up.Seal(nil, nil, []byte("only one way"), nil)
	_, err := down.Open(nil, nil, sealed, nil)
	require.Error(t, err)
}

func TestAEADMaxNonceForcesARotation(t *testing.T) {
	key := randomKey(t)
	a := NewAEAD([]byte("ctx"), key, true)
	copy(a.Nonce[:], MaxNonce)
	before := a.Nonce

	sealed := a.Seal(nil, nil, []byte("rotated"), nil)
	require.NotEqual(t, before, a.Nonce)

	// The keystream must have come from the rotated counter, so the old one is
	// useless for reading it back.
	_, err := NewAEAD([]byte("ctx"), key, true).Open(nil, MaxNonce, sealed, nil)
	require.Error(t, err)
}

func TestAEADSupportsChaCha20Poly1305(t *testing.T) {
	key := randomKey(t)
	sealer := NewAEAD([]byte("ctx"), key, false)
	sealed := sealer.Seal(nil, nil, []byte("chacha"), nil)

	opened, err := NewAEAD([]byte("ctx"), key, false).Open(nil, nil, sealed, nil)
	require.NoError(t, err)
	require.Equal(t, "chacha", string(opened))
}

// Padding is a user-supplied string whose shapes are validated at parse time,
// so the rejections below are the contract a malformed config relies on.
func TestParsePadding(t *testing.T) {
	for name, bad := range map[string]string{
		"missing separator":   "100-111",
		"empty first field":   "-111-1111",
		"empty second field":  "100--1111",
		"empty third field":   "100-111-",
		"first length small":  "35-111-1111",
		"first padding small": "99-16-17",
		"not a number":        "abc-111-1111",
		"total too long":      "100-65536-65536.100-65536-65536.75-0-111.",
	} {
		t.Run(name, func(t *testing.T) {
			var lens, gaps [][3]int
			require.Error(t, ParsePadding(bad, &lens, &gaps))
		})
	}

	var lens, gaps [][3]int
	require.NoError(t, ParsePadding("", &lens, &gaps))
	require.Empty(t, lens)
	require.Empty(t, gaps)

	require.NoError(t, ParsePadding("100-111-1111.75-0-111.50-0-3333", &lens, &gaps))
	require.Equal(t, [][3]int{{100, 111, 1111}, {50, 0, 3333}}, lens)
	require.Equal(t, [][3]int{{75, 0, 111}}, gaps)
}

func TestParsePaddingRejectsATrailingDot(t *testing.T) {
	var lens, gaps [][3]int
	// A trailing dot leaves an empty segment behind, which the split rules out.
	require.Error(t, ParsePadding("100-111-1111.", &lens, &gaps))
}

// with no configuration at all the defaults must produce a fragmentable
// plan, otherwise a server would emit one giant write.
func TestCreatPaddingFallsBackToTheDefaultPlan(t *testing.T) {
	length, lens, gaps := CreatPadding(nil, nil)
	require.Positive(t, length)
	require.Len(t, lens, 2)
	require.Len(t, gaps, 1)
	require.Equal(t, length, lens[0]+lens[1])
}

func TestCreatPaddingRespectsTheProbabilityZero(t *testing.T) {
	length, lens, gaps := CreatPadding([][3]int{{0, 10, 20}}, [][3]int{{0, 0, 100}})
	require.Equal(t, 0, length)
	require.Equal(t, []int{0}, lens)
	require.Equal(t, []int{0}, lens)
	require.Len(t, gaps, 1)
}

// NewClient and NewServer are the config entry points, so their rejections are
// what stops a typo from reaching a handshake.
func TestNewClientRejectsMalformedEncryption(t *testing.T) {
	for name, bad := range map[string]string{
		"unknown scheme":   "aes-256-gcm",
		"unknown mode":     "mlkem768x25519plus.ellswift.0rtt.key",
		"unknown rtt":      "mlkem768x25519plus.native.2rtt.key",
		"short key length": "mlkem768x25519plus.native.0rtt.AAAA",
		"bad base64":       "mlkem768x25519plus.native.0rtt.!!!!",
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient(bad)
			require.Error(t, err)
			require.Nil(t, client)
		})
	}

	client, err := NewClient("")
	require.NoError(t, err)
	require.Nil(t, client)

	client, err = NewClient("none")
	require.NoError(t, err)
	require.Nil(t, client)
}

func TestNewServerRejectsMalformedDecryption(t *testing.T) {
	for name, bad := range map[string]string{
		"unknown scheme": "aes-256-gcm",
		"unknown mode":   "mlkem768x25519plus.ellswift.600s.key",
		"bad seconds":    "mlkem768x25519plus.native.abc.key",
		"bad range":      "mlkem768x25519plus.native.600-abc.key",
		"short key":      "mlkem768x25519plus.native.600s.AAAA",
	} {
		t.Run(name, func(t *testing.T) {
			server, err := NewServer(bad)
			require.Error(t, err)
			require.Nil(t, server)
		})
	}

	server, err := NewServer("")
	require.NoError(t, err)
	require.Nil(t, server)

	server, err = NewServer("none")
	require.NoError(t, err)
	require.Nil(t, server)
}

func TestNewClientInitRejectsAReuse(t *testing.T) {
	client := clientInstance(t)
	require.Error(t, client.Init(client.NfsPKeysBytes, 0, 0, ""))
}

func TestNewServerInitRejectsAReuse(t *testing.T) {
	server := serverInstance(t, "600s")
	require.Error(t, server.Init(server.NfsPKeysBytes, 0, 600, 0, ""))
}

func TestNewClientInitRejectsAnEmptyKeySet(t *testing.T) {
	client := &ClientInstance{}
	require.Error(t, client.Init(nil, 0, 0, ""))
}

func TestNewServerInitRejectsAnEmptyKeySet(t *testing.T) {
	server := &ServerInstance{}
	require.Error(t, server.Init(nil, 0, 0, 0, ""))
}

func TestNewServerCloseStopsTheSweeper(t *testing.T) {
	server := serverInstance(t, "600s")
	require.NoError(t, server.Close())
	require.True(t, server.Closed)
}

func TestHandshakeRejectsAnUninitializedInstance(t *testing.T) {
	client := clientInstance(t)
	client.NfsPKeys = nil
	_, err := client.Handshake(nil)
	require.ErrorContains(t, err, "uninitialized")

	server := &ServerInstance{}
	_, err = server.Handshake(nil, nil)
	require.ErrorContains(t, err, "uninitialized")
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	return randomBytes(t, 32)
}

// The generator is the same code path a user's config goes through, so these
// fixtures are what a real handshake would carry.
func clientInstance(t *testing.T) *ClientInstance {
	t.Helper()
	_, clientBase64, _, err := GenMLKEM768("")
	require.NoError(t, err)
	_, passwordBase64, _, err := GenX25519("")
	require.NoError(t, err)

	client, err := NewClient("mlkem768x25519plus.native.0rtt." + clientBase64 + "." + passwordBase64)
	require.NoError(t, err)
	require.NotNil(t, client)
	return client
}

func serverInstance(t *testing.T, seconds string) *ServerInstance {
	t.Helper()
	_, _, _, err := GenX25519("")
	require.NoError(t, err)
	seedBase64, _, _, err := GenMLKEM768("")
	require.NoError(t, err)
	privateKeyBase64, _, _, err := GenX25519("")
	require.NoError(t, err)

	server, err := NewServer("mlkem768x25519plus.native." + seconds + "." + privateKeyBase64 + "." + seedBase64)
	require.NoError(t, err)
	require.NotNil(t, server)
	return server
}

var _ = strings.Split
