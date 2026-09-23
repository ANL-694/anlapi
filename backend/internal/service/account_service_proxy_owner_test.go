package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type ownedProxyServiceRepoStub struct {
	ProxyRepository
	proxies      map[int64]*Proxy
	accountCount int64
	deleted      bool
}

func (s *ownedProxyServiceRepoStub) GetOwnedByID(_ context.Context, ownerUserID, id int64) (*Proxy, error) {
	p := s.proxies[id]
	if p == nil || p.OwnerUserID == nil || *p.OwnerUserID != ownerUserID {
		return nil, ErrProxyNotFound
	}
	copy := *p
	return &copy, nil
}

func (s *ownedProxyServiceRepoStub) ListOwnedByUserID(_ context.Context, ownerUserID int64) ([]ProxyWithAccountCount, error) {
	result := make([]ProxyWithAccountCount, 0)
	for _, p := range s.proxies {
		if p.OwnerUserID != nil && *p.OwnerUserID == ownerUserID {
			result = append(result, ProxyWithAccountCount{Proxy: *p})
		}
	}
	return result, nil
}

func (s *ownedProxyServiceRepoStub) CountByOwnerUserID(_ context.Context, ownerUserID int64) (int64, error) {
	var count int64
	for _, p := range s.proxies {
		if p.OwnerUserID != nil && *p.OwnerUserID == ownerUserID {
			count++
		}
	}
	return count, nil
}

func (s *ownedProxyServiceRepoStub) CountOwnedAccountsByProxyID(_ context.Context, ownerUserID, proxyID int64) (int64, error) {
	p, err := s.GetOwnedByID(context.Background(), ownerUserID, proxyID)
	if err != nil {
		return 0, err
	}
	_ = p
	return s.accountCount, nil
}

func (s *ownedProxyServiceRepoStub) Create(_ context.Context, p *Proxy) error {
	copy := *p
	copy.ID = int64(len(s.proxies) + 1)
	s.proxies[copy.ID] = &copy
	p.ID = copy.ID
	return nil
}

func (s *ownedProxyServiceRepoStub) Update(_ context.Context, p *Proxy) error {
	copy := *p
	s.proxies[p.ID] = &copy
	return nil
}

func (s *ownedProxyServiceRepoStub) DeleteOwned(_ context.Context, ownerUserID, id int64) error {
	if _, err := s.GetOwnedByID(context.Background(), ownerUserID, id); err != nil {
		return err
	}
	delete(s.proxies, id)
	s.deleted = true
	return nil
}

func TestAccountServiceOwnedProxyScope(t *testing.T) {
	owner := int64(11)
	other := int64(22)
	repo := &ownedProxyServiceRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "mine", Protocol: "https", Host: "mine.test", Port: 443, Status: StatusActive, OwnerUserID: &owner},
			2: {ID: 2, Name: "other", Protocol: "https", Host: "other.test", Port: 443, Status: StatusActive, OwnerUserID: &other},
		},
	}
	svc := NewAccountService(nil, nil)
	svc.SetProxyRepository(repo)

	proxies, err := svc.ListOwnedProxies(context.Background(), owner)
	require.NoError(t, err)
	require.Len(t, proxies, 1)
	require.Equal(t, int64(1), proxies[0].ID)

	_, err = svc.UpdateOwnedProxy(context.Background(), owner, 2, UpdateProxyRequest{})
	require.ErrorIs(t, err, ErrProxyNotFound)

	created, err := svc.CreateOwnedProxy(context.Background(), owner, CreateProxyRequest{
		Name: "new", Protocol: "https", Host: "new.test", Port: 443,
	})
	require.NoError(t, err)
	require.Equal(t, &owner, created.OwnerUserID)

	repo.accountCount = 1
	err = svc.DeleteOwnedProxy(context.Background(), owner, 1)
	require.ErrorIs(t, err, ErrProxyInUse)
	repo.accountCount = 0
	err = svc.DeleteOwnedProxy(context.Background(), owner, 1)
	require.NoError(t, err)
	require.True(t, repo.deleted)
}

func TestAccountServiceOwnedProxyValidation(t *testing.T) {
	owner := int64(11)
	repo := &ownedProxyServiceRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Status: StatusActive, OwnerUserID: &owner},
		},
	}
	svc := NewAccountService(nil, nil)
	svc.SetProxyRepository(repo)

	id, err := svc.ValidateOwnedProxyID(context.Background(), owner, ptrInt64(1))
	require.NoError(t, err)
	require.Equal(t, int64(1), *id)

	_, err = svc.ValidateOwnedProxyID(context.Background(), 99, ptrInt64(1))
	require.ErrorIs(t, err, ErrProxyNotFound)
}

func TestAccountServiceOwnedProxyProbeFailsClosedWhenProberMissing(t *testing.T) {
	owner := int64(11)
	repo := &ownedProxyServiceRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Status: StatusActive, OwnerUserID: &owner},
		},
	}
	svc := NewAccountService(nil, nil)
	svc.SetProxyRepository(repo)

	_, err := svc.TestOwnedProxy(context.Background(), owner, 1)
	require.ErrorIs(t, err, ErrProxyProbeUnavailable)
	_, err = svc.CheckOwnedProxyQuality(context.Background(), owner, 1)
	require.ErrorIs(t, err, ErrProxyProbeUnavailable)
}

func ptrInt64(v int64) *int64 { return &v }
