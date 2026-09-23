export default {
  revenue: {
    title: '营收统计',
    description: '按支付、实际用量和账号成本查看营收与利润。',
    filters: {
      startDate: '开始日期',
      endDate: '结束日期',
      granularity: '粒度',
      day: '按天',
      hour: '按小时'
    },
    metrics: {
      netPaid: '净收款',
      consumedRevenue: '实际消耗',
      accountCost: '账号成本',
      netProfit: '估算净利润',
      ownerCredit: '账号主分成',
      paidOrders: '{count} 笔已支付订单',
      requests: '{count} 次请求',
      tokens: '{count} Token',
      grossProfit: '毛利 {value}',
      platformFee: '平台分成 {value}'
    },
    trend: {
      title: '趋势明细',
      date: '时间'
    },
    breakdowns: {
      users: '用户排行',
      accounts: '账号排行'
    },
    table: {
      name: '名称',
      requests: '请求数',
      tokens: 'Token',
      revenue: '消耗收入',
      net: '净利润'
    },
    empty: '当前筛选条件下没有数据',
    invalidDateRange: '结束日期不能早于开始日期',
    loadFailed: '加载营收统计失败'
  }
}
