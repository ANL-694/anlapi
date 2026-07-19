package service

import (
	"context"
	"net/http"
)

func setOpenAIChatGPTAccountHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
		headers.Set("chatgpt-account-id", chatgptAccountID)
	}
	if account.IsChatGPTAccountFedRAMP() {
		headers.Set("x-openai-fedramp", "true")
	} else {
		headers.Del("x-openai-fedramp")
	}
}

func resolveAndSetOpenAIChatGPTAccountHeaders(ctx context.Context, repo AccountRepository, headers http.Header, account *Account) error {
	credentialAccount, err := resolveCredentialAccount(ctx, repo, account)
	if err != nil {
		return err
	}
	setOpenAIChatGPTAccountHeaders(headers, credentialAccount)
	return nil
}
