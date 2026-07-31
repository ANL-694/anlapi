package service

func calculatePrivateGroupCommissionCost(p *postUsageBillingParams) float64 {
	if p == nil || p.Cost == nil || !p.IsSubscriptionBill || p.APIKey == nil || p.APIKey.Group == nil {
		return 0
	}
	if !p.APIKey.Group.IsUserPrivateScope() || p.Cost.ActualCost <= 0 {
		return 0
	}
	rate := p.PrivateGroupCommissionRate
	if rate <= 0 {
		return 0
	}
	if rate > 1 {
		rate = 1
	}
	return p.Cost.ActualCost * rate
}
