package service

import (
	"strings"
	"testing"

	"anlapi/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestCodexVersionConstantsConsistency(t *testing.T) {
	require.Equal(t, codexCLIVersion, openAICodexProbeVersion)
	require.Contains(t, codexCLIUserAgent, openai.CodexDefaultOriginator+"/"+codexCLIVersion)
	require.True(t, strings.Contains(codexCLIUserAgent, codexCLIVersion))
}
