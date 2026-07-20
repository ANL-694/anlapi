<template>
  <AppLayout>
    <UiPage width="wide">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <UserDashboardQuickActions :user="user" :stats="stats" />
        <UserDashboardStats :stats="stats" :user="user" :is-simple="authStore.isSimpleMode" />
        <UserDashboardCharts
          v-model:startDate="startDate"
          v-model:endDate="endDate"
          v-model:granularity="granularity"
          :loading="loadingCharts"
          :trend="trendData"
          :models="modelStats"
          @dateRangeChange="loadTimeRangeData"
          @granularityChange="loadTimeRangeData"
          @refresh="refreshAll"
        />
        <UserDashboardPlatformUsage :stats="stats" />
        <UserDashboardPlatformQuotas :quotas="platformQuotas" :loading="loadingPlatformQuotas" />
        <UserAccountSharingStats
          :stats="accountSharingStats"
          :loading="loadingAccountSharing"
          :error="accountSharingError"
          :page="accountSharingPage"
          :page-size="accountSharingPageSize"
          @update:page="handleAccountSharingPageChange"
          @update:pageSize="handleAccountSharingPageSizeChange"
        />
        <UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" />
      </template>
    </UiPage>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { usageAPI, type AccountSharingDashboardStats, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { userAPI } from '@/api/user'
import { PLATFORM_QUOTA_PLATFORMS, type PlatformQuotaRecord } from '@/api/platformQuotas'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { UiPage } from '@/ui'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import UserDashboardPlatformUsage from '@/components/user/dashboard/UserDashboardPlatformUsage.vue'
import UserDashboardPlatformQuotas from '@/components/user/dashboard/UserDashboardPlatformQuotas.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserAccountSharingStats from '@/components/user/dashboard/UserAccountSharingStats.vue'
import type { ModelStat, TrendDataPoint, UsageLog } from '@/types'

const authStore = useAuthStore()
const user = computed(() => authStore.user)

const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const loadingAccountSharing = ref(false)
const loadingPlatformQuotas = ref(false)
const accountSharingError = ref('')

const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const accountSharingStats = ref<AccountSharingDashboardStats | null>(null)
const platformQuotas = ref<PlatformQuotaRecord[]>([])
const accountSharingPage = ref(1)
const accountSharingPageSize = ref(20)

const formatLocalDate = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
const startDate = ref(formatLocalDate(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLocalDate(new Date()))
const granularity = ref<'day' | 'hour'>('day')

const loadStats = async () => {
  loading.value = true
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  try {
    const [trend, models] = await Promise.all([
      usageAPI.getDashboardTrend({
        start_date: startDate.value,
        end_date: endDate.value,
        granularity: granularity.value
      }),
      usageAPI.getDashboardModels({
        start_date: startDate.value,
        end_date: endDate.value
      })
    ])
    trendData.value = trend.trend || []
    modelStats.value = models.models || []
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadAccountSharing = async () => {
  loadingAccountSharing.value = true
  accountSharingError.value = ''
  try {
    accountSharingStats.value = await usageAPI.getDashboardAccountSharing({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      account_page: accountSharingPage.value,
      account_page_size: accountSharingPageSize.value
    })
  } catch (error: any) {
    console.error('Failed to load account sharing stats:', error)
    accountSharingStats.value = null
    accountSharingError.value = error?.message || 'Failed to load account sharing stats'
  } finally {
    loadingAccountSharing.value = false
  }
}

const loadRecent = async () => {
  loadingUsage.value = true
  try {
    const res = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = res.items.slice(0, 5)
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

const loadPlatformQuotas = async () => {
  loadingPlatformQuotas.value = true
  try {
    const response = await userAPI.getPlatformQuotas()
    platformQuotas.value = [...(response.platform_quotas || [])].sort(
      (a, b) => PLATFORM_QUOTA_PLATFORMS.indexOf(a.platform) - PLATFORM_QUOTA_PLATFORMS.indexOf(b.platform)
    )
  } catch (error) {
    console.error('Failed to load platform quotas:', error)
    platformQuotas.value = []
  } finally {
    loadingPlatformQuotas.value = false
  }
}

const loadTimeRangeData = () => {
  accountSharingPage.value = 1
  void loadCharts()
  void loadAccountSharing()
}

const handleAccountSharingPageChange = (page: number) => {
  accountSharingPage.value = page
  void loadAccountSharing()
}

const handleAccountSharingPageSizeChange = (pageSize: number) => {
  accountSharingPageSize.value = pageSize
  accountSharingPage.value = 1
  void loadAccountSharing()
}

const refreshAll = () => {
  void loadStats()
  void loadCharts()
  void loadAccountSharing()
  void loadRecent()
  void loadPlatformQuotas()
}

onMounted(() => {
  refreshAll()
})
</script>
