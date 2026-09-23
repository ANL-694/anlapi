export default {
  accountShare: {
    title: 'Account Sharing Revenue',
    description: 'Manage account-sharing revenue policies and audit applied settlements.',
    tabs: {
      policies: 'Revenue Policies',
      settlements: 'Settlement Audit'
    },
    scopes: {
      global: 'Global',
      platform: 'Platform',
      group: 'Group',
      account: 'Account'
    },
    targets: {
      all: 'All accounts'
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled',
      applied: 'Applied',
      frozen: 'Frozen',
      reversed: 'Reversed'
    },
    filters: {
      allScopes: 'All scopes',
      allStatus: 'All statuses',
      platform: 'Filter by platform',
      search: 'Search request, user, account, or platform',
      startDate: 'Start date',
      endDate: 'End date'
    },
    columns: {
      scope: 'Policy scope',
      platform: 'Platform',
      ownerRatio: 'Owner share',
      inviteRatio: 'Invite share',
      totalRatio: 'Total ratio',
      version: 'Version',
      effectiveAt: 'Effective at',
      status: 'Status',
      actions: 'Actions',
      request: 'Request',
      consumer: 'Consumer',
      owner: 'Owner',
      inviter: 'Inviter',
      account: 'Account / model',
      consumerCharge: 'Consumer charge',
      accountCost: 'Account cost'
    },
    settlement: {
      consumer: 'Consumer'
    },
    actions: {
      createPolicy: 'Create policy',
      editPolicy: 'Edit policy',
      enablePolicy: 'Enable policy',
      disablePolicy: 'Disable policy'
    },
    dialog: {
      createTitle: 'Create revenue policy',
      editTitle: 'Edit revenue policy',
      deleteTitle: 'Delete revenue policy',
      deleteMessage: 'The policy will no longer be used for future requests. Existing settlements keep their snapshots. Continue?'
    },
    form: {
      scope: 'Scope',
      platform: 'Platform identifier',
      scopeId: 'Scope ID',
      ownerRatio: 'Owner share ratio',
      inviteRatio: 'Invite share ratio',
      ratioHint: 'The two ratios cannot exceed 100% together. Changes apply to future requests; historical settlements keep their snapshots.',
      ratioError: 'Enter ratios from 0 to 100; their sum cannot exceed 100%.',
      platformRequired: 'A platform identifier is required for a platform policy.',
      scopeIdRequired: 'A valid scope ID is required for group or account policies.',
      effectiveAt: 'Effective at',
      enabled: 'Policy enabled'
    },
    messages: {
      saved: 'Revenue policy saved.',
      updated: 'Revenue policy updated.',
      deleted: 'Revenue policy deleted.'
    },
    errors: 'Account-sharing revenue operation failed'
  }
}
