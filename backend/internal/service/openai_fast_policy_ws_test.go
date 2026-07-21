package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWSResponseCreate_PreservesServiceTierAndNormalizesFastAlias(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	cases := []struct {
		name  string
		frame string
		want  string
	}{
		{name: "fast", frame: `{"type":"response.create","model":"gpt-5.5","service_tier":"fast"}`, want: OpenAIFastTierPriority},
		{name: "priority", frame: `{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`, want: OpenAIFastTierPriority},
		{name: "flex", frame: `{"type":"response.create","model":"gpt-5.5","service_tier":"flex"}`, want: OpenAIFastTierFlex},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", []byte(tc.frame))
			require.NoError(t, err)
			require.Nil(t, blocked)
			require.Equal(t, tc.want, gjson.GetBytes(updated, "service_tier").String())
		})
	}
}

func TestWSResponseCreate_DoesNotMutateOtherFramesOrUnsupportedValues(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, frame := range [][]byte{
		[]byte(`{"type":"response.cancel","service_tier":"fast"}`),
		[]byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"turbo"}`),
		[]byte(`{"type":"response.create","model":"gpt-5.5","service_tier":1}`),
		[]byte(`{"type":"response.create","model":"gpt-5.5"}`),
	} {
		updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
		require.NoError(t, err)
		require.Nil(t, blocked)
		require.Equal(t, string(frame), string(updated))
	}
}

func TestWSResponseCreate_LegacyBlockingSettingsCannotBlockFastMode(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"priority"}`)

	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
}

func TestWSResponseCreate_BillingSeesTheTierSentUpstream(t *testing.T) {
	svc := &OpenAIGatewayService{}
	frame := []byte(`{"type":"response.create","model":"gpt-5.5","service_tier":"fast"}`)

	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), &Account{Platform: PlatformOpenAI}, "gpt-5.5", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	tier := extractOpenAIServiceTierFromBody(updated)
	require.NotNil(t, tier)
	require.Equal(t, OpenAIFastTierPriority, *tier)
}
