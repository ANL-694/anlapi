package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIFastPolicy_DoesNotLocallyRestrictLegacyRules(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, tier := range []string{OpenAIFastTierPriority, OpenAIFastTierFlex} {
		action, message := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", tier)
		require.Equal(t, BetaPolicyActionPass, action)
		require.Empty(t, message)
	}

	legacy := &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier: OpenAIFastTierPriority,
		Action:      BetaPolicyActionBlock,
		Scope:       BetaPolicyScopeAll,
	}}}
	action, message := evaluateOpenAIFastPolicyWithSettings(legacy, 1, account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)
	require.Empty(t, message)
}

func TestApplyOpenAIFastPolicyToBody_PreservesSupportedTiers(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "fast alias", body: `{"model":"gpt-5.5","service_tier":"fast"}`, want: OpenAIFastTierPriority},
		{name: "mixed case fast alias", body: `{"model":"gpt-5.5","service_tier":"  Fast  "}`, want: OpenAIFastTierPriority},
		{name: "priority", body: `{"model":"gpt-5.5","service_tier":"priority"}`, want: OpenAIFastTierPriority},
		{name: "flex", body: `{"model":"gpt-5.5","service_tier":"flex"}`, want: OpenAIFastTierFlex},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", []byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.want, gjson.GetBytes(updated, "service_tier").String())
		})
	}
}

func TestApplyOpenAIFastPolicyToBody_LeavesUnsupportedAndNonStringValuesUntouched(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-5.5"}`),
		[]byte(`{"model":"gpt-5.5","service_tier":"turbo"}`),
		[]byte(`{"model":"gpt-5.5","service_tier":1}`),
		[]byte(`{"model":"gpt-5.5","service_tier":null}`),
	} {
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err)
		require.Equal(t, string(body), string(updated))
	}
}
