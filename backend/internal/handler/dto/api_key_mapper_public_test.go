package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyFromServicePublicMasksLongKey(t *testing.T) {
	src := &service.APIKey{Key: "fixture-test-1234567890-example"}

	public := APIKeyFromServicePublic(src)
	full := APIKeyFromService(src)

	require.Equal(t, "fixtur...mple", public.Key)
	require.Equal(t, src.Key, full.Key)
	require.NotContains(t, public.Key, "1234567890")
}

func TestAPIKeyFromServicePublicMasksShortAndEmptyKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "short", key: "short-key", want: "shor***"},
		{name: "very short", key: "abc", want: "abc***"},
		{name: "empty", key: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			public := APIKeyFromServicePublic(&service.APIKey{Key: tt.key})
			require.Equal(t, tt.want, public.Key)
		})
	}
}

func TestAPIKeyFromServicePublicIsIdempotent(t *testing.T) {
	for _, key := range []string{"sk-tes...mple", "shor***"} {
		public := APIKeyFromServicePublic(&service.APIKey{Key: key})
		require.Equal(t, key, public.Key)
	}
}

func TestAPIKeyFromServicePublicNilIsSafe(t *testing.T) {
	require.Nil(t, APIKeyFromServicePublic(nil))
}
