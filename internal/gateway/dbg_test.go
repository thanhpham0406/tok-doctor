package gateway

import (
	"fmt"
	"testing"
)

func TestDebugTotalInput(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":800,"output_tokens":30}}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	fmt.Printf("pu: %+v\n", pu)
	obs := ProviderUsageToObserved(pu)
	fmt.Printf("Observed RawInput=%d Cached=%d CacheCreation=%d TotalInput=%d Output=%d Total=%d\n", obs.RawInput, obs.Cached, obs.CacheCreation, obs.TotalInput, obs.Output, obs.Total)
}
