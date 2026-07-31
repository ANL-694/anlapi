<template>
  <AppLayout>
    <UiPage width="wide" density="compact">
      <UiPageHeader
        :title="t('httpStatusCodes.title')"
        :description="t('httpStatusCodes.description')"
      />

      <div class="http-status-note" role="note">
        <span class="http-status-note-mark">i</span>
        <p>{{ t('httpStatusCodes.note') }}</p>
      </div>

      <div class="http-status-groups">
        <section
          v-for="group in statusGroups"
          :key="group.range"
          class="http-status-group"
        >
          <button
            type="button"
            class="http-status-group-header"
            :aria-expanded="isGroupExpanded(group.range)"
            :aria-controls="`http-status-group-${group.range}`"
            @click="toggleGroup(group.range)"
          >
            <div class="http-status-group-title">
              <span class="http-status-range" :class="`http-status-range--${group.tone}`">
                {{ group.range }}
              </span>
              <span class="http-status-group-title-text" role="heading" aria-level="2">
                {{ getText(group.title) }}
              </span>
            </div>
            <span class="http-status-group-meta">
              <span class="http-status-count">
                {{ group.entries.length }} {{ t('httpStatusCodes.codes') }}
              </span>
              <Icon
                name="chevronDown"
                size="sm"
                class="http-status-chevron"
                :class="{ 'http-status-chevron-expanded': isGroupExpanded(group.range) }"
                aria-hidden="true"
              />
            </span>
          </button>

          <div
            v-if="isGroupExpanded(group.range)"
            :id="`http-status-group-${group.range}`"
            class="http-status-list"
          >
            <article
              v-for="status in group.entries"
              :key="status.code"
              class="http-status-row"
            >
              <code class="http-status-code">{{ status.code }}</code>
              <div class="min-w-0">
                <h3 class="http-status-name">{{ getText(status.name) }}</h3>
                <p class="http-status-description">{{ getText(status.description) }}</p>
              </div>
            </article>
          </div>
        </section>
      </div>
    </UiPage>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { UiPage, UiPageHeader } from '@/ui'

interface LocalizedText {
  zh: string
  en: string
}

interface StatusCodeEntry {
  code: number
  name: LocalizedText
  description: LocalizedText
}

interface StatusCodeGroup {
  range: string
  tone: 'info' | 'success' | 'redirect' | 'client' | 'server'
  title: LocalizedText
  entries: StatusCodeEntry[]
}

const { t, locale } = useI18n()
const expandedGroups = ref<Set<string>>(new Set())

const text = (zh: string, en: string): LocalizedText => ({ zh, en })
const status = (code: number, name: LocalizedText, description: LocalizedText): StatusCodeEntry => ({
  code,
  name,
  description,
})

const statusGroups: StatusCodeGroup[] = [
  {
    range: '1xx',
    tone: 'info',
    title: text('信息响应', 'Informational responses'),
    entries: [
      status(100, text('Continue', 'Continue'), text('服务器已收到请求头，客户端可继续发送请求体。', 'The server received the request headers; the client may continue sending the request body.')),
      status(101, text('Switching Protocols', 'Switching Protocols'), text('正在切换协议，例如升级到 WebSocket。', 'The server is switching protocols, such as upgrading to WebSocket.')),
      status(102, text('Processing', 'Processing'), text('服务器正在处理请求，但尚未完成（WebDAV）。', 'The server is processing the request but has not completed it yet (WebDAV).')),
      status(103, text('Early Hints', 'Early Hints'), text('提供早期响应头提示，客户端可以提前加载资源。', 'Provides early response header hints so the client can preload resources.')),
    ],
  },
  {
    range: '2xx',
    tone: 'success',
    title: text('成功', 'Success'),
    entries: [
      status(200, text('OK', 'OK'), text('请求成功，返回所请求的数据。', 'The request succeeded and the requested data is returned.')),
      status(201, text('Created', 'Created'), text('请求成功，并已创建新资源。', 'The request succeeded and a new resource was created.')),
      status(202, text('Accepted', 'Accepted'), text('已接受请求，但尚未处理完成。', 'The request was accepted but has not been completed yet.')),
      status(203, text('Non-Authoritative Information', 'Non-Authoritative Information'), text('返回的信息可能来自本地或第三方副本，并非源站权威响应。', 'The returned information may come from a local or third-party copy rather than the origin server.')),
      status(204, text('No Content', 'No Content'), text('请求成功，但没有返回内容。', 'The request succeeded but has no response content.')),
      status(205, text('Reset Content', 'Reset Content'), text('请求成功，并要求客户端重置文档视图或表单。', 'The request succeeded and the client should reset the document view or form.')),
      status(206, text('Partial Content', 'Partial Content'), text('返回请求资源的一部分，常用于断点续传。', 'Part of the requested resource is returned, commonly for resumable downloads.')),
      status(207, text('Multi-Status', 'Multi-Status'), text('返回多个独立操作的状态（WebDAV）。', 'Returns the status of multiple independent operations (WebDAV).')),
      status(208, text('Already Reported', 'Already Reported'), text('资源已在此前响应中列出（WebDAV）。', 'The resource was already listed in an earlier response (WebDAV).')),
      status(226, text('IM Used', 'IM Used'), text('服务器已完成实例操作并返回增量表示（Delta Encoding）。', 'The server completed an instance manipulation and returned a delta representation (Delta Encoding).')),
    ],
  },
  {
    range: '3xx',
    tone: 'redirect',
    title: text('重定向', 'Redirection'),
    entries: [
      status(300, text('Multiple Choices', 'Multiple Choices'), text('存在多个可选表示或位置，需要客户端选择。', 'Multiple representations or locations are available and the client must choose one.')),
      status(301, text('Moved Permanently', 'Moved Permanently'), text('资源已永久移动到新 URL。', 'The resource has permanently moved to a new URL.')),
      status(302, text('Found', 'Found'), text('资源暂时位于其他 URL；部分客户端或历史实现可能将后续请求改为 GET，需严格保留方法时使用 307。', 'The resource is temporarily at another URL; some clients or legacy implementations may change the follow-up request to GET. Use 307 when the method must be preserved.')),
      status(303, text('See Other', 'See Other'), text('客户端应使用 GET 请求另一个 URL。', 'The client should use GET to request another URL.')),
      status(304, text('Not Modified', 'Not Modified'), text('条件请求表明资源未修改，可以使用缓存副本；它不是普通重定向。', 'A conditional request shows that the resource has not changed and a cached copy may be used; this is not an ordinary redirect.')),
      status(305, text('Use Proxy', 'Use Proxy'), text('必须通过代理访问；此状态码已废弃。', 'The resource must be accessed through a proxy; this status code is deprecated.')),
      status(306, text('Unused', 'Unused'), text('未使用或保留的状态码。', 'An unused or reserved status code.')),
      status(307, text('Temporary Redirect', 'Temporary Redirect'), text('临时重定向，必须保持请求方法和请求体。', 'A temporary redirect that must preserve the request method and body.')),
      status(308, text('Permanent Redirect', 'Permanent Redirect'), text('永久重定向，必须保持请求方法和请求体。', 'A permanent redirect that must preserve the request method and body.')),
    ],
  },
  {
    range: '4xx',
    tone: 'client',
    title: text('客户端错误', 'Client errors'),
    entries: [
      status(400, text('Bad Request', 'Bad Request'), text('请求语法、参数或格式无效。', 'The request syntax, parameters, or format is invalid.')),
      status(401, text('Unauthorized', 'Unauthorized'), text('缺少有效身份认证，或提供的认证失败。', 'Valid authentication is missing or the supplied authentication failed.')),
      status(402, text('Payment Required', 'Payment Required'), text('为将来使用预留；少数服务用于支付相关限制。', 'Reserved for future use; some services use it for payment-related restrictions.')),
      status(403, text('Forbidden', 'Forbidden'), text('服务器理解请求，但拒绝授权访问。', 'The server understood the request but refuses to authorize access.')),
      status(404, text('Not Found', 'Not Found'), text('服务器找不到请求的资源。', 'The server cannot find the requested resource.')),
      status(405, text('Method Not Allowed', 'Method Not Allowed'), text('请求方法不被目标资源允许。', 'The request method is not allowed for the target resource.')),
      status(406, text('Not Acceptable', 'Not Acceptable'), text('无法生成满足 Accept 等内容协商条件的响应。', 'The server cannot produce a response that satisfies the content negotiation conditions such as Accept.')),
      status(407, text('Proxy Authentication Required', 'Proxy Authentication Required'), text('需要先通过代理服务器认证。', 'Authentication with the proxy server is required first.')),
      status(408, text('Request Timeout', 'Request Timeout'), text('服务器等待请求超时。', 'The server timed out waiting for the request.')),
      status(409, text('Conflict', 'Conflict'), text('请求与资源当前状态冲突。', 'The request conflicts with the current state of the resource.')),
      status(410, text('Gone', 'Gone'), text('资源已永久删除且不再可用。', 'The resource has been permanently removed and is no longer available.')),
      status(411, text('Length Required', 'Length Required'), text('请求必须包含有效 Content-Length。', 'The request must include a valid Content-Length header.')),
      status(412, text('Precondition Failed', 'Precondition Failed'), text('请求头中的前置条件不满足。', 'A precondition in the request headers was not met.')),
      status(413, text('Content Too Large', 'Content Too Large'), text('请求体超过服务器愿意或能够处理的大小（旧称 Payload Too Large）。', 'The request content is larger than the server is willing or able to process (formerly Payload Too Large).')),
      status(414, text('URI Too Long', 'URI Too Long'), text('请求目标 URI 过长。', 'The request target URI is too long.')),
      status(415, text('Unsupported Media Type', 'Unsupported Media Type'), text('请求格式或 Content-Type 不受支持。', 'The request format or Content-Type is not supported.')),
      status(416, text('Range Not Satisfiable', 'Range Not Satisfiable'), text('Range 请求的范围无法满足。', 'The requested range cannot be satisfied.')),
      status(417, text('Expectation Failed', 'Expectation Failed'), text('Expect 请求头中的期望无法满足。', 'The expectation in the Expect request header cannot be met.')),
      status(418, text("I'm a teapot", "I'm a teapot"), text('历史玩笑状态码，不代表通用业务语义。', 'A historical joke status code with no general business meaning.')),
      status(421, text('Misdirected Request', 'Misdirected Request'), text('请求被导向无法生成响应的服务器。', 'The request was directed to a server that cannot produce a response.')),
      status(422, text('Unprocessable Content', 'Unprocessable Content'), text('语法正确但语义无效，无法处理（旧称 Unprocessable Entity，常见于 WebDAV）。', 'The syntax is valid but the content is semantically invalid and cannot be processed (formerly Unprocessable Entity, common in WebDAV).')),
      status(423, text('Locked', 'Locked'), text('目标资源被锁定（WebDAV）。', 'The target resource is locked (WebDAV).')),
      status(424, text('Failed Dependency', 'Failed Dependency'), text('依赖的其他请求失败（WebDAV）。', 'A dependent request failed (WebDAV).')),
      status(425, text('Too Early', 'Too Early'), text('服务器拒绝处理可能被重放的请求。', 'The server refuses to process a request that may be replayed.')),
      status(426, text('Upgrade Required', 'Upgrade Required'), text('客户端需要升级到其他协议。', 'The client needs to upgrade to a different protocol.')),
      status(428, text('Precondition Required', 'Precondition Required'), text('服务器要求请求包含条件头。', 'The server requires the request to include a conditional header.')),
      status(429, text('Too Many Requests', 'Too Many Requests'), text('请求过于频繁，触发限流。', 'Too many requests were sent in a given period and rate limiting was triggered.')),
      status(431, text('Request Header Fields Too Large', 'Request Header Fields Too Large'), text('请求头字段总大小过大。', 'The request header fields are too large.')),
      status(451, text('Unavailable For Legal Reasons', 'Unavailable For Legal Reasons'), text('因法律原因无法提供资源。', 'The resource is unavailable for legal reasons.')),
    ],
  },
  {
    range: '5xx',
    tone: 'server',
    title: text('服务器错误', 'Server errors'),
    entries: [
      status(500, text('Internal Server Error', 'Internal Server Error'), text('服务器遇到未预期的内部错误。', 'The server encountered an unexpected internal error.')),
      status(501, text('Not Implemented', 'Not Implemented'), text('服务器不支持完成请求所需的功能。', 'The server does not support the functionality required to fulfill the request.')),
      status(502, text('Bad Gateway', 'Bad Gateway'), text('网关或代理从上游收到无效响应。', 'The gateway or proxy received an invalid response from the upstream server.')),
      status(503, text('Service Unavailable', 'Service Unavailable'), text('服务暂时无法处理请求，常见于过载或维护。', 'The service is temporarily unable to handle the request, commonly because of overload or maintenance.')),
      status(504, text('Gateway Timeout', 'Gateway Timeout'), text('网关或代理等待上游响应超时。', 'The gateway or proxy timed out waiting for an upstream response.')),
      status(505, text('HTTP Version Not Supported', 'HTTP Version Not Supported'), text('服务器不支持请求使用的 HTTP 版本。', 'The server does not support the HTTP version used in the request.')),
      status(506, text('Variant Also Negotiates', 'Variant Also Negotiates'), text('内容协商配置导致内部循环。', 'The content negotiation configuration creates an internal loop.')),
      status(507, text('Insufficient Storage', 'Insufficient Storage'), text('服务器无法存储完成请求所需的表示（WebDAV）。', 'The server cannot store the representation needed to complete the request (WebDAV).')),
      status(508, text('Loop Detected', 'Loop Detected'), text('处理请求时检测到无限循环（WebDAV）。', 'An infinite loop was detected while processing the request (WebDAV).')),
      status(510, text('Not Extended', 'Not Extended'), text('请求需要额外扩展才能完成。', 'The request requires further extensions to be fulfilled.')),
      status(511, text('Network Authentication Required', 'Network Authentication Required'), text('客户端需要先完成网络认证，例如 Wi-Fi 登录页。', 'The client needs to complete network authentication first, such as on a Wi-Fi login page.')),
    ],
  },
]

function getText(value: LocalizedText): string {
  return locale.value.startsWith('zh') ? value.zh : value.en
}

function isGroupExpanded(range: string): boolean {
  return expandedGroups.value.has(range)
}

function toggleGroup(range: string): void {
  const next = new Set(expandedGroups.value)
  if (next.has(range)) {
    next.delete(range)
  } else {
    next.add(range)
  }
  expandedGroups.value = next
}
</script>

<style scoped>
.http-status-note {
  display: flex;
  align-items: flex-start;
  gap: 0.625rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid color-mix(in srgb, var(--ui-accent) 24%, var(--ui-border));
  border-radius: var(--ui-radius-md);
  background: color-mix(in srgb, var(--ui-accent) 8%, var(--ui-surface));
  color: var(--ui-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.5;
}

.http-status-note-mark {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border: 1px solid color-mix(in srgb, var(--ui-accent) 40%, transparent);
  border-radius: 999px;
  color: var(--ui-accent);
  font-size: 0.75rem;
  font-weight: 700;
}

.http-status-groups {
  display: grid;
  gap: 1rem;
}

.http-status-group {
  overflow: hidden;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-lg);
  background: var(--ui-surface);
}

.http-status-group-header {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid var(--ui-border);
  background: var(--ui-surface-muted);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color 160ms ease;
}

.http-status-group-header:hover {
  background: color-mix(in srgb, var(--ui-surface-muted) 78%, var(--ui-accent));
}

.http-status-group-header:focus-visible {
  outline: 2px solid var(--ui-accent);
  outline-offset: -2px;
}

.http-status-group-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.75rem;
}

.http-status-group-title-text {
  min-width: 0;
  color: var(--ui-text);
  font-size: 0.9375rem;
  font-weight: 600;
}

.http-status-range {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  min-height: 1.625rem;
  padding: 0.125rem 0.5rem;
  border-radius: 999px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.75rem;
  font-weight: 700;
}

.http-status-range--info { color: #2563eb; background: #dbeafe; }
.http-status-range--success { color: #047857; background: #d1fae5; }
.http-status-range--redirect { color: #b45309; background: #fef3c7; }
.http-status-range--client { color: #c2410c; background: #ffedd5; }
.http-status-range--server { color: #be123c; background: #ffe4e6; }

.http-status-count {
  flex: 0 0 auto;
  color: var(--ui-text-tertiary);
  font-size: 0.75rem;
}

.http-status-group-meta {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.5rem;
}

.http-status-chevron {
  color: var(--ui-text-tertiary);
  transition: transform 160ms ease;
}

.http-status-chevron-expanded {
  transform: rotate(180deg);
}

.http-status-row {
  display: grid;
  grid-template-columns: 4.5rem minmax(0, 1fr);
  gap: 0.875rem;
  padding: 0.875rem 1rem;
}

.http-status-row + .http-status-row {
  border-top: 1px solid var(--ui-border-subtle);
}

.http-status-code {
  align-self: start;
  padding-top: 0.1rem;
  color: var(--ui-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  font-weight: 700;
}

.http-status-name {
  color: var(--ui-text);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1.4;
}

.http-status-description {
  margin-top: 0.25rem;
  color: var(--ui-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.55;
}

@media (max-width: 640px) {
  .http-status-group-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 0.5rem;
  }

  .http-status-row {
    grid-template-columns: 3.75rem minmax(0, 1fr);
    gap: 0.625rem;
    padding: 0.75rem;
  }
}
</style>
