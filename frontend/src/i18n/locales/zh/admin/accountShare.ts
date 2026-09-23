export default {
  accountShare: {
    title: '账号共享收益',
    description: '管理账号共享分成策略，并审计已落账的结算明细。',
    tabs: {
      policies: '分成策略',
      settlements: '结算审计'
    },
    scopes: {
      global: '全局',
      platform: '平台',
      group: '分组',
      account: '账号'
    },
    targets: {
      all: '全部账号'
    },
    status: {
      enabled: '启用',
      disabled: '停用',
      applied: '已入账',
      frozen: '已冻结',
      reversed: '已冲正'
    },
    filters: {
      allScopes: '全部范围',
      allStatus: '全部状态',
      platform: '平台筛选',
      search: '搜索请求、用户、账号或平台',
      startDate: '开始日期',
      endDate: '结束日期'
    },
    columns: {
      scope: '策略范围',
      platform: '平台',
      ownerRatio: '账号主分成',
      inviteRatio: '邀请分成',
      totalRatio: '合计比例',
      version: '版本',
      effectiveAt: '生效时间',
      status: '状态',
      actions: '操作',
      request: '请求',
      consumer: '消费者',
      owner: '账号主',
      inviter: '邀请人',
      account: '账号 / 模型',
      consumerCharge: '用户扣费',
      accountCost: '账号成本'
    },
    settlement: {
      consumer: '消费者'
    },
    actions: {
      createPolicy: '新建策略',
      editPolicy: '编辑策略',
      enablePolicy: '启用策略',
      disablePolicy: '停用策略'
    },
    dialog: {
      createTitle: '新建分成策略',
      editTitle: '编辑分成策略',
      deleteTitle: '删除分成策略',
      deleteMessage: '删除后该策略不再参与后续请求的解析，已经完成的结算不会被改写。确定继续吗？'
    },
    form: {
      scope: '适用范围',
      platform: '平台标识',
      scopeId: '范围 ID',
      ownerRatio: '账号主分成比例',
      inviteRatio: '邀请人分成比例',
      ratioHint: '两个比例合计不能超过 100%。比例只影响后续请求，历史结算保留快照。',
      ratioError: '请输入 0 到 100 之间的比例，且两个比例合计不能超过 100%。',
      platformRequired: '平台策略必须填写平台标识。',
      scopeIdRequired: '分组或账号策略必须填写有效的范围 ID。',
      effectiveAt: '生效时间',
      enabled: '策略启用'
    },
    messages: {
      saved: '分成策略已保存。',
      updated: '分成策略已更新。',
      deleted: '分成策略已删除。'
    },
    errors: '账户共享收益操作失败'
  }
}
