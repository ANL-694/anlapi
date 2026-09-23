<template>
  <AppLayout>
    <UiPage width="wide" density="compact" class="dashboard-page dashboard-page--user">
      <div v-if="loading && !stats" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <div v-if="statsError" class="mb-4">
          <ErrorState :retry="true" @retry="loadStats" />
        </div>
        <UserDashboardHeroActions
          :balance="user?.balance || 0"
          :total-api-keys="stats.total_api_keys"
          :active-api-keys="stats.active_api_keys"
          :today-requests="stats.today_requests"
        />
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" />
        <UserDashboardPlatformUsage :stats="stats" />
        <UserDashboardPlatformQuotas
          :quotas="platformQuotas"
          :loading="loadingPlatformQuotas"
          :error="platformQuotasError"
          @retry="loadPlatformQuotas"
        />
        <UserDashboardAccountSharing
          :data="accountSharing"
          :loading="loadingAccountSharing"
          :error="accountSharingError"
          :page="accountSharingPage"
          :page-size="accountSharingPageSize"
          @update:page="handleAccountSharingPageChange"
          @update:pageSize="handleAccountSharingPageSizeChange"
        />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :error="chartsError" :trend="trendData" :models="modelStats" @dateRangeChange="reloadPeriodData" @granularityChange="reloadPeriodData" @refresh="refreshAll" @retry="loadCharts" />
        <UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" :error="recentUsageError" @retry="loadRecent" />
      </template>
      <ErrorState v-else :retry="true" @retry="loadStats" />
    </UiPage>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type AccountSharingDashboardStats, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'; import ErrorState from '@/components/common/ErrorState.vue'; import { UiPage } from '@/ui'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardHeroActions from '@/components/user/dashboard/UserDashboardHeroActions.vue'
import UserDashboardPlatformUsage from '@/components/user/dashboard/UserDashboardPlatformUsage.vue'; import UserDashboardPlatformQuotas from '@/components/user/dashboard/UserDashboardPlatformQuotas.vue'
import UserDashboardAccountSharing from '@/components/user/dashboard/UserDashboardAccountSharing.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false); const loadingPlatformQuotas = ref(false); const loadingAccountSharing = ref(false); const accountSharingError = ref('')
const statsError = ref(false); const chartsError = ref(false); const recentUsageError = ref(false); const platformQuotasError = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
const accountSharing = ref<AccountSharingDashboardStats | null>(null)
const accountSharingPage = ref(1); const accountSharingPageSize = ref(20)

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))); const endDate = ref(formatDateLocalInput(new Date())); const granularity = ref('day')

const loadStats = async () => {
  loading.value = true
  statsError.value = false
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    statsError.value = true
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  chartsError.value = false
  try {
    const res = await Promise.all([
      usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }),
      usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })
    ])
    trendData.value = res[0].trend || []
    modelStats.value = res[1].models || []
  } catch (error) {
    chartsError.value = true
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadRecent = async () => {
  loadingUsage.value = true
  recentUsageError.value = false
  try {
    const res = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = res.items.slice(0, 5)
  } catch (error) {
    recentUsageError.value = true
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

const loadPlatformQuotas = async () => {
  loadingPlatformQuotas.value = true
  platformQuotasError.value = false
  try {
    const data = await getMyPlatformQuotas()
    platformQuotas.value = data.platform_quotas ?? []
  } catch (error) {
    platformQuotasError.value = true
    console.warn('Failed to load platform quotas:', error)
  } finally {
    loadingPlatformQuotas.value = false
  }
}
const loadAccountSharing = async () => { loadingAccountSharing.value = true; accountSharingError.value = ''; try { accountSharing.value = await usageAPI.getDashboardAccountSharing({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any, account_page: accountSharingPage.value, account_page_size: accountSharingPageSize.value }) } catch (error: any) { console.warn('Failed to load account sharing:', error); accountSharing.value = null; accountSharingError.value = error?.message || 'Failed to load account sharing stats' } finally { loadingAccountSharing.value = false } }
const reloadPeriodData = () => { accountSharingPage.value = 1; loadCharts(); loadRecent(); loadAccountSharing() }
const handleAccountSharingPageChange = (page: number) => { accountSharingPage.value = page; loadAccountSharing() }
const handleAccountSharingPageSizeChange = (pageSize: number) => { accountSharingPageSize.value = pageSize; accountSharingPage.value = 1; loadAccountSharing() }
const refreshAll = () => { loadStats(); reloadPeriodData(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>
