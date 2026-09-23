export default {
  revenue: {
    title: 'Revenue',
    description: 'Review revenue and profit from payments, actual usage, and account cost.',
    filters: {
      startDate: 'Start date',
      endDate: 'End date',
      granularity: 'Granularity',
      day: 'Daily',
      hour: 'Hourly'
    },
    metrics: {
      netPaid: 'Net paid',
      consumedRevenue: 'Consumed revenue',
      accountCost: 'Account cost',
      netProfit: 'Estimated net profit',
      ownerCredit: 'Owner credit',
      paidOrders: '{count} paid orders',
      requests: '{count} requests',
      tokens: '{count} tokens',
      grossProfit: 'Gross profit {value}',
      platformFee: 'Platform fee {value}'
    },
    trend: {
      title: 'Trend',
      date: 'Date'
    },
    breakdowns: {
      users: 'Top users',
      accounts: 'Top accounts'
    },
    table: {
      name: 'Name',
      requests: 'Requests',
      tokens: 'Tokens',
      revenue: 'Consumed revenue',
      net: 'Net profit'
    },
    empty: 'No data for the selected range',
    invalidDateRange: 'End date cannot be before start date',
    loadFailed: 'Failed to load revenue'
  }
}
