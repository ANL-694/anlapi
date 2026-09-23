package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type withdrawalRepoStub struct {
	submitInput WithdrawalSubmitInput
	submitReq   *WithdrawalRequest
	rejectReq   *WithdrawalRequest
	lastNote    string
}

func (r *withdrawalRepoStub) Submit(_ context.Context, input WithdrawalSubmitInput) (*WithdrawalRequest, error) {
	r.submitInput = input
	return r.submitReq, nil
}
func (r *withdrawalRepoStub) Cancel(context.Context, int64, int64, string) (*WithdrawalRequest, error) {
	panic("unexpected Cancel")
}
func (r *withdrawalRepoStub) GetByID(context.Context, int64) (*WithdrawalRequest, error) {
	panic("unexpected GetByID")
}
func (r *withdrawalRepoStub) ListByUser(context.Context, int64, int, int) ([]WithdrawalRequest, int64, error) {
	panic("unexpected ListByUser")
}
func (r *withdrawalRepoStub) ListAdmin(context.Context, WithdrawalListParams) ([]WithdrawalRequest, int64, error) {
	panic("unexpected ListAdmin")
}
func (r *withdrawalRepoStub) Settle(context.Context, int64, int64, string) (*WithdrawalRequest, error) {
	panic("unexpected Settle")
}
func (r *withdrawalRepoStub) Reject(_ context.Context, _ int64, _ int64, note string) (*WithdrawalRequest, error) {
	r.lastNote = note
	return r.rejectReq, nil
}
func (r *withdrawalRepoStub) ReceiptCodeInUse(context.Context, string) (bool, error) {
	panic("unexpected ReceiptCodeInUse")
}

type withdrawalAuthInvalidatorStub struct{ userIDs []int64 }

func (s *withdrawalAuthInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string) {}

func (s *withdrawalAuthInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *withdrawalAuthInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestWithdrawalServiceSubmitValidatesBeforeRepository(t *testing.T) {
	repo := &withdrawalRepoStub{}
	svc := NewWithdrawalService(repo, nil, nil, nil)

	_, err := svc.Submit(context.Background(), WithdrawalSubmitInput{UserID: 1, Amount: 0.999, PaymentMethod: "alipay"})
	require.ErrorIs(t, err, ErrWithdrawalAmountInvalid)
	require.Zero(t, repo.submitInput)

	_, err = svc.Submit(context.Background(), WithdrawalSubmitInput{UserID: 1, Amount: 1, PaymentMethod: "unknown"})
	require.ErrorIs(t, err, ErrReceiptCodePaymentMethodInvalid)
	require.Zero(t, repo.submitInput)
}

func TestWithdrawalServiceSubmitNormalizesAndInvalidatesBalance(t *testing.T) {
	repo := &withdrawalRepoStub{submitReq: &WithdrawalRequest{ID: 3, UserID: 42}}
	invalidator := &withdrawalAuthInvalidatorStub{}
	svc := NewWithdrawalService(repo, invalidator, nil, nil)

	got, err := svc.Submit(context.Background(), WithdrawalSubmitInput{UserID: 42, Amount: 12.5, PaymentMethod: " ALIPAY "})
	require.NoError(t, err)
	require.Equal(t, repo.submitReq, got)
	require.Equal(t, WithdrawalSubmitInput{UserID: 42, Amount: 12.5, PaymentMethod: "alipay"}, repo.submitInput)
	require.Equal(t, []int64{42}, invalidator.userIDs)
}

func TestWithdrawalServiceAdminRejectTrimsNoteAndInvalidatesBalance(t *testing.T) {
	repo := &withdrawalRepoStub{rejectReq: &WithdrawalRequest{ID: 8, UserID: 77}}
	invalidator := &withdrawalAuthInvalidatorStub{}
	svc := NewWithdrawalService(repo, invalidator, nil, nil)

	got, err := svc.AdminReject(context.Background(), 8, 9, "  invalid receipt  ")
	require.NoError(t, err)
	require.Equal(t, repo.rejectReq, got)
	require.Equal(t, "invalid receipt", repo.lastNote)
	require.Equal(t, []int64{77}, invalidator.userIDs)
}

var _ WithdrawalRepository = (*withdrawalRepoStub)(nil)
var _ APIKeyAuthCacheInvalidator = (*withdrawalAuthInvalidatorStub)(nil)
