package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	dbent "anlapi/ent"
	"anlapi/internal/payment"
	infraerrors "anlapi/internal/pkg/errors"
)

func TestShouldUseAlipayMobilePrecreate(t *testing.T) {
	t.Parallel()

	enabled := &PaymentConfig{AlipayMobilePrecreateDeepLink: true}
	officialAlipay := &payment.InstanceSelection{ProviderKey: payment.TypeAlipay}
	tests := []struct {
		name string
		req  CreateOrderRequest
		cfg  *PaymentConfig
		sel  *payment.InstanceSelection
		want bool
	}{
		{name: "balance hosted mobile official", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance, PaymentSource: PaymentSourceHostedRedirect}, cfg: enabled, sel: officialAlipay, want: true},
		{name: "subscription default source mobile official", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeSubscription}, cfg: enabled, sel: officialAlipay, want: true},
		{name: "desktop remains wap", req: CreateOrderRequest{OrderType: payment.OrderTypeBalance}, cfg: enabled, sel: officialAlipay},
		{name: "switch disabled", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance}, cfg: &PaymentConfig{}, sel: officialAlipay},
		{name: "easypay remains unchanged", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance}, cfg: enabled, sel: &payment.InstanceSelection{ProviderKey: payment.TypeEasyPay}},
		{name: "shop remains unchanged", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeShop}, cfg: enabled, sel: officialAlipay},
		{name: "non hosted source remains unchanged", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance, PaymentSource: "api"}, cfg: enabled, sel: officialAlipay},
		{name: "nil config", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance}, sel: officialAlipay},
		{name: "nil selection", req: CreateOrderRequest{IsMobile: true, OrderType: payment.OrderTypeBalance}, cfg: enabled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldUseAlipayMobilePrecreate(test.req, test.cfg, test.sel); got != test.want {
				t.Fatalf("shouldUseAlipayMobilePrecreate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestShouldExposeAlipayMobilePrecreateDeepLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		precreateEnabled bool
		qrCode           string
		want             bool
	}{
		{name: "enabled with dynamic qr", precreateEnabled: true, qrCode: "https://qr.alipay.example/dynamic", want: true},
		{name: "enabled with blank qr", precreateEnabled: true, qrCode: "  "},
		{name: "disabled with qr", qrCode: "https://qr.alipay.example/dynamic"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldExposeAlipayMobilePrecreateDeepLink(test.precreateEnabled, test.qrCode); got != test.want {
				t.Fatalf("shouldExposeAlipayMobilePrecreateDeepLink() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestIsOfficialAlipayProviderInstance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		instance *dbent.PaymentProviderInstance
		want     bool
	}{
		{name: "nil", instance: nil},
		{name: "official", instance: &dbent.PaymentProviderInstance{ProviderKey: payment.TypeAlipay}, want: true},
		{name: "normalized official", instance: &dbent.PaymentProviderInstance{ProviderKey: " ALIPAY "}, want: true},
		{name: "easypay", instance: &dbent.PaymentProviderInstance{ProviderKey: payment.TypeEasyPay}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isOfficialAlipayProviderInstance(test.instance); got != test.want {
				t.Fatalf("isOfficialAlipayProviderInstance() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBuildCreateOrderResponseDefaultsToOrderCreated(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	resp := buildCreateOrderResponse(
		&dbent.PaymentOrder{
			ID:         42,
			Amount:     12.34,
			FeeRate:    0.03,
			ExpiresAt:  expiresAt,
			OutTradeNo: "sub2_42",
		},
		CreateOrderRequest{PaymentType: payment.TypeWxpay},
		12.71,
		&payment.InstanceSelection{PaymentMode: "qrcode"},
		&payment.CreatePaymentResponse{
			TradeNo: "sub2_42",
			QRCode:  "weixin://wxpay/bizpayurl?pr=test",
		},
		payment.CreatePaymentResultOrderCreated,
	)

	if resp.ResultType != payment.CreatePaymentResultOrderCreated {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultOrderCreated)
	}
	if resp.OutTradeNo != "sub2_42" {
		t.Fatalf("out_trade_no = %q, want %q", resp.OutTradeNo, "sub2_42")
	}
	if resp.QRCode != "weixin://wxpay/bizpayurl?pr=test" {
		t.Fatalf("qr_code = %q, want %q", resp.QRCode, "weixin://wxpay/bizpayurl?pr=test")
	}
	if resp.JSAPI != nil || resp.JSAPIPayload != nil {
		t.Fatal("order_created response should not include jsapi payload")
	}
	if !resp.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at = %v, want %v", resp.ExpiresAt, expiresAt)
	}
}

func TestBuildCreateOrderResponseCopiesJSAPIPayload(t *testing.T) {
	t.Parallel()

	jsapiPayload := &payment.WechatJSAPIPayload{
		AppID:     "wx123",
		TimeStamp: "1712345678",
		NonceStr:  "nonce-123",
		Package:   "prepay_id=wx123",
		SignType:  "RSA",
		PaySign:   "signed-payload",
	}
	resp := buildCreateOrderResponse(
		&dbent.PaymentOrder{
			ID:         88,
			Amount:     66.88,
			FeeRate:    0.01,
			ExpiresAt:  time.Date(2026, 4, 16, 13, 0, 0, 0, time.UTC),
			OutTradeNo: "sub2_88",
		},
		CreateOrderRequest{PaymentType: payment.TypeWxpay},
		67.55,
		&payment.InstanceSelection{PaymentMode: "popup"},
		&payment.CreatePaymentResponse{
			TradeNo:    "sub2_88",
			ResultType: payment.CreatePaymentResultJSAPIReady,
			JSAPI:      jsapiPayload,
		},
		payment.CreatePaymentResultJSAPIReady,
	)

	if resp.ResultType != payment.CreatePaymentResultJSAPIReady {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultJSAPIReady)
	}
	if resp.JSAPI == nil || resp.JSAPIPayload == nil {
		t.Fatal("expected jsapi payload aliases to be populated")
	}
	if resp.JSAPI != jsapiPayload || resp.JSAPIPayload != jsapiPayload {
		t.Fatal("expected jsapi aliases to preserve the original pointer")
	}
}

func TestBuildProviderReturnURLForSelectionCompactsLongEasyPayURL(t *testing.T) {
	t.Parallel()

	got, err := buildProviderReturnURLForSelection(
		"https://api.example.com/payment/result",
		42,
		"sub2_20260703abcdef",
		strings.Repeat("x", 320),
		&payment.InstanceSelection{ProviderKey: payment.TypeEasyPay},
	)
	if err != nil {
		t.Fatalf("buildProviderReturnURLForSelection returned error: %v", err)
	}
	if len(got) > easyPayReturnURLMaxLength {
		t.Fatalf("return_url length = %d, want <= %d: %s", len(got), easyPayReturnURLMaxLength, got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse return_url: %v", err)
	}
	if parsed.Query().Get("resume_token") != "" {
		t.Fatalf("resume_token should be omitted for compact EasyPay return_url: %s", got)
	}
	if parsed.Query().Get("order_id") != "42" {
		t.Fatalf("order_id = %q, want 42", parsed.Query().Get("order_id"))
	}
	if parsed.Query().Get("out_trade_no") != "sub2_20260703abcdef" {
		t.Fatalf("out_trade_no = %q", parsed.Query().Get("out_trade_no"))
	}
	if parsed.Query().Get("status") != "success" {
		t.Fatalf("status = %q, want success", parsed.Query().Get("status"))
	}
}

func TestBuildProviderReturnURLForSelectionKeepsLongURLForOtherProviders(t *testing.T) {
	t.Parallel()

	resumeToken := strings.Repeat("x", 320)
	got, err := buildProviderReturnURLForSelection(
		"https://api.example.com/payment/result",
		42,
		"sub2_20260703abcdef",
		resumeToken,
		&payment.InstanceSelection{ProviderKey: payment.TypeWxpay},
	)
	if err != nil {
		t.Fatalf("buildProviderReturnURLForSelection returned error: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse return_url: %v", err)
	}
	if parsed.Query().Get("resume_token") != resumeToken {
		t.Fatalf("resume_token was not preserved for non-EasyPay provider")
	}
}

func TestBuildPaymentSubjectDefaultsToANLAPIBalanceRecharge(t *testing.T) {
	t.Parallel()

	got := (*PaymentService)(nil).buildPaymentSubject(
		CreateOrderRequest{OrderType: payment.OrderTypeBalance},
		nil,
		5,
		&PaymentConfig{},
		&payment.InstanceSelection{ProviderKey: payment.TypeEasyPay},
	)

	if got != "ANLAPI 余额充值 5.00 CNY" {
		t.Fatalf("subject = %q, want %q", got, "ANLAPI 余额充值 5.00 CNY")
	}
}

func TestBuildPaymentSubjectKeepsConfiguredProductNamePrefixSuffix(t *testing.T) {
	t.Parallel()

	got := (*PaymentService)(nil).buildPaymentSubject(
		CreateOrderRequest{OrderType: payment.OrderTypeBalance},
		nil,
		5,
		&PaymentConfig{
			ProductNamePrefix: "自定义",
			ProductNameSuffix: "充值",
		},
		&payment.InstanceSelection{ProviderKey: payment.TypeEasyPay},
	)

	if got != "自定义 5.00 充值" {
		t.Fatalf("subject = %q, want %q", got, "自定义 5.00 充值")
	}
}

func TestMaybeBuildWeChatOAuthRequiredResponse(t *testing.T) {
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", "0123456789abcdef0123456789abcdef")

	svc := newWeChatPaymentOAuthTestService(map[string]string{
		SettingKeyWeChatConnectEnabled:             "true",
		SettingKeyWeChatConnectAppID:               "wx123456",
		SettingKeyWeChatConnectAppSecret:           "wechat-secret",
		SettingKeyWeChatConnectMode:                "mp",
		SettingKeyWeChatConnectScopes:              "snsapi_base",
		SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
		SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
	})

	resp, err := svc.maybeBuildWeChatOAuthRequiredResponse(context.Background(), CreateOrderRequest{
		UserID:          123,
		Amount:          12.5,
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
		SrcURL:          "https://merchant.example/payment?from=wechat",
		OrderType:       payment.OrderTypeBalance,
	}, 12.5, 12.88, 0.03)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected oauth_required response, got nil")
	}
	if resp.ResultType != payment.CreatePaymentResultOAuthRequired {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultOAuthRequired)
	}
	if resp.OAuth == nil {
		t.Fatal("expected oauth payload, got nil")
	}
	if resp.OAuth.AppID != "wx123456" {
		t.Fatalf("appid = %q, want %q", resp.OAuth.AppID, "wx123456")
	}
	if resp.OAuth.Scope != "snsapi_base" {
		t.Fatalf("scope = %q, want %q", resp.OAuth.Scope, "snsapi_base")
	}
	if resp.OAuth.RedirectURL != "/auth/wechat/payment/callback" {
		t.Fatalf("redirect_url = %q, want %q", resp.OAuth.RedirectURL, "/auth/wechat/payment/callback")
	}
	parsedAuthorizeURL, err := url.Parse(resp.OAuth.AuthorizeURL)
	if err != nil {
		t.Fatalf("parse authorize_url: %v", err)
	}
	if parsedAuthorizeURL.Path != "/api/v1/auth/oauth/wechat/payment/start" {
		t.Fatalf("authorize_url path = %q", parsedAuthorizeURL.Path)
	}
	contextToken := parsedAuthorizeURL.Query().Get("context_token")
	if contextToken == "" {
		t.Fatalf("authorize_url missing context_token: %q", resp.OAuth.AuthorizeURL)
	}
	claims, err := svc.paymentResume().ParseWeChatPaymentOAuthContextToken(contextToken)
	if err != nil {
		t.Fatalf("parse context token: %v", err)
	}
	if claims.UserID != 123 {
		t.Fatalf("context user id = %d, want 123", claims.UserID)
	}
	if claims.Amount != "12.5" || claims.OrderType != payment.OrderTypeBalance || claims.PaymentType != payment.TypeWxpay || claims.RedirectTo != "/purchase?from=wechat" {
		t.Fatalf("unexpected context claims: %+v", claims)
	}
}

func TestMaybeBuildWeChatOAuthRequiredResponseRequiresMPConfigInWeChat(t *testing.T) {
	t.Parallel()

	svc := newWeChatPaymentOAuthTestService(nil)

	resp, err := svc.maybeBuildWeChatOAuthRequiredResponse(context.Background(), CreateOrderRequest{
		UserID:          123,
		Amount:          12.5,
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
		SrcURL:          "https://merchant.example/payment?from=wechat",
		OrderType:       payment.OrderTypeBalance,
	}, 12.5, 12.88, 0.03)
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := infraerrors.FromError(err)
	if appErr.Reason != "WECHAT_PAYMENT_MP_NOT_CONFIGURED" {
		t.Fatalf("reason = %q, want %q", appErr.Reason, "WECHAT_PAYMENT_MP_NOT_CONFIGURED")
	}
}

func TestMaybeBuildWeChatOAuthRequiredResponseRequiresResumeSigningKey(t *testing.T) {
	t.Parallel()

	svc := &PaymentService{
		configService: &PaymentConfigService{
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingKeyWeChatConnectEnabled:             "true",
				SettingKeyWeChatConnectAppID:               "wx123456",
				SettingKeyWeChatConnectAppSecret:           "wechat-secret",
				SettingKeyWeChatConnectMode:                "mp",
				SettingKeyWeChatConnectScopes:              "snsapi_base",
				SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
				SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
			}},
			// Intentionally missing payment resume signing key.
			encryptionKey: nil,
		},
	}

	resp, err := svc.maybeBuildWeChatOAuthRequiredResponse(context.Background(), CreateOrderRequest{
		UserID:          123,
		Amount:          12.5,
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
		SrcURL:          "https://merchant.example/payment?from=wechat",
		OrderType:       payment.OrderTypeBalance,
	}, 12.5, 12.88, 0.03)
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := infraerrors.FromError(err)
	if appErr.Reason != "PAYMENT_RESUME_NOT_CONFIGURED" {
		t.Fatalf("reason = %q, want %q", appErr.Reason, "PAYMENT_RESUME_NOT_CONFIGURED")
	}
}

func TestMaybeBuildWeChatOAuthRequiredResponseFallsBackToConfiguredLegacySigningKey(t *testing.T) {
	svc := &PaymentService{
		configService: &PaymentConfigService{
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingKeyWeChatConnectEnabled:             "true",
				SettingKeyWeChatConnectAppID:               "wx123456",
				SettingKeyWeChatConnectAppSecret:           "wechat-secret",
				SettingKeyWeChatConnectMode:                "mp",
				SettingKeyWeChatConnectScopes:              "snsapi_base",
				SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
				SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
			}},
			// Legacy stable signing key remains available for no-config upgrade compatibility.
			encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		},
	}

	resp, err := svc.maybeBuildWeChatOAuthRequiredResponse(context.Background(), CreateOrderRequest{
		UserID:          123,
		Amount:          12.5,
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
		SrcURL:          "https://merchant.example/payment?from=wechat",
		OrderType:       payment.OrderTypeBalance,
	}, 12.5, 12.88, 0.03)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected oauth-required response, got nil")
	}
	if resp.ResultType != payment.CreatePaymentResultOAuthRequired {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultOAuthRequired)
	}
	if resp.OAuth == nil || strings.TrimSpace(resp.OAuth.AuthorizeURL) == "" {
		t.Fatalf("expected oauth redirect payload, got %+v", resp.OAuth)
	}
}

func TestMaybeBuildWeChatOAuthRequiredResponseForSelectionSkipsEasyPayProvider(t *testing.T) {
	svc := newWeChatPaymentOAuthTestService(map[string]string{
		SettingKeyWeChatConnectEnabled:             "true",
		SettingKeyWeChatConnectAppID:               "wx123456",
		SettingKeyWeChatConnectAppSecret:           "wechat-secret",
		SettingKeyWeChatConnectMode:                "mp",
		SettingKeyWeChatConnectScopes:              "snsapi_base",
		SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
		SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
	})

	resp, err := svc.maybeBuildWeChatOAuthRequiredResponseForSelection(context.Background(), CreateOrderRequest{
		Amount:          12.5,
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
		OrderType:       payment.OrderTypeBalance,
	}, 12.5, 12.88, 0.03, &payment.InstanceSelection{
		ProviderKey: payment.TypeEasyPay,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
}

func TestComputeValidityDaysSupportsSingularAndPluralUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		days int
		unit string
		want int
	}{
		{name: "days", days: 1, unit: "days", want: 1},
		{name: "week", days: 1, unit: "week", want: 7},
		{name: "weeks", days: 2, unit: "weeks", want: 14},
		{name: "month", days: 1, unit: "month", want: 30},
		{name: "months", days: 1, unit: "months", want: 30},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := psComputeValidityDays(tt.days, tt.unit); got != tt.want {
				t.Fatalf("psComputeValidityDays(%d, %q) = %d, want %d", tt.days, tt.unit, got, tt.want)
			}
		})
	}
}

func newWeChatPaymentOAuthTestService(values map[string]string) *PaymentService {
	return &PaymentService{
		configService: &PaymentConfigService{
			settingRepo:   &paymentConfigSettingRepoStub{values: values},
			encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		},
	}
}
