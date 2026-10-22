package vision

import (
	"testing"

	"github.com/metacubex/mihomo/common/buf"
	"github.com/stretchr/testify/require"
)

// ApplyPadding runs on every filtered write, so its debug log must not build
// its arguments while debug logging is off: three boxed values and the call's
// variadic slice are two allocations per write.
func TestApplyPaddingBuildsNoLogArgumentsWithDebugOff(t *testing.T) {
	backing := make([]byte, 4096)
	buffer := buf.With(backing)
	buffer.Advance(PaddingHeaderLen)

	allocations := testing.AllocsPerRun(100, func() {
		buffer.Reset()
		buffer.Advance(PaddingHeaderLen)
		ApplyPadding(buffer, commandPaddingContinue, nil, true)
	})
	require.Zero(t, allocations)
}
