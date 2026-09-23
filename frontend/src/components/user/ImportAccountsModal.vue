<template>
  <CredentialImportModal
    :show="show"
    :title="t('userAccounts.importTitle')"
    :hint="t('userAccounts.importHint')"
    :warning="t('userAccounts.importWarning')"
    form-id="user-import-accounts-form"
    :importer="importPersonalCredentials"
    @close="$emit('close')"
    @imported="$emit('imported', $event)"
  />
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { accountsAPI } from '@/api'
import CredentialImportModal from '@/components/account/CredentialImportModal.vue'
import type { ImportCredentialContentsResponse } from '@/api/accounts'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported', payload?: { close: boolean }): void
}

defineProps<Props>()
defineEmits<Emits>()

const { t } = useI18n()

function importPersonalCredentials(
  contents: string[],
  options?: { kiroConfigImport?: boolean }
): Promise<ImportCredentialContentsResponse> {
  return accountsAPI.importCredentialContents({
    contents,
    kiro_config_import: options?.kiroConfigImport,
    share_mode: 'private'
  })
}
</script>
