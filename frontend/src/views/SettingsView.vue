<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { Message } from "@arco-design/web-vue";
import { IconSave } from "@arco-design/web-vue/es/icon";
import { apiFetch, jsonRequest } from "../api";
import { useAuthStore } from "../stores/auth";
import type { AgentAPIKey, AppSettings } from "../types";

const auth = useAuthStore();
const loading = ref(false);
const saving = ref(false);
const creatingKey = ref(false);
const keyName = ref("");
const latestRawKey = ref("");
const agentKeys = ref<AgentAPIKey[]>([]);
const form = reactive<AppSettings>({
  smtp_host: "",
  smtp_port: "587",
  smtp_username: "",
  smtp_password: "",
  smtp_from: "",
  admin_notify_email: "",
  llm_base_url: "",
  llm_api_key: "",
  llm_model: "",
  llm_system_prompt: "",
  embedding_base_url: "",
  embedding_api_key: "",
  embedding_model: "",
  embedding_dimensions: 0,
  embedding_timeout_ms: 15000,
  semantic_search_enabled: false,
  comment_review_mode: "manual_all",
  about_name: "",
  about_tagline: "",
  about_avatar_url: "",
  about_email: "",
  about_github_url: "",
  about_bio: "",
  about_repo_count: "",
  about_star_count: "",
  about_fork_count: "",
  about_friend_links: "",
});

const reviewModeLabelMap: Record<AppSettings["comment_review_mode"], string> = {
  manual_all: "全部人工审核",
  auto_approve_ai_passed: "AI 通过后自动放行",
};

const reviewModeHintMap: Record<AppSettings["comment_review_mode"], string> = {
  manual_all: "所有评论都进入人工审核队列，适合风险控制优先的站点。",
  auto_approve_ai_passed: "只有 AI 明确判定为通过的评论会直接展示，其余评论仍进入人工审核。",
};

function hasText(value: string | null | undefined) {
  return Boolean(String(value || "").trim());
}

const reviewModeLabel = computed(() => reviewModeLabelMap[form.comment_review_mode]);
const reviewModeHint = computed(() => reviewModeHintMap[form.comment_review_mode]);
const mailReady = computed(() =>
  hasText(form.smtp_host) && hasText(form.smtp_from) && hasText(form.admin_notify_email),
);
const llmReady = computed(() =>
  hasText(form.llm_base_url) && hasText(form.llm_model) && hasText(form.llm_api_key),
);
const semanticReady = computed(() =>
  form.semantic_search_enabled &&
  hasText(form.embedding_base_url) &&
  hasText(form.embedding_model) &&
  hasText(form.embedding_api_key),
);
const agentKeySummary = computed(() =>
  agentKeys.value.length === 0 ? "未创建" : `${agentKeys.value.length} 个可用凭证`,
);
const aboutFriendCount = computed(() =>
  form.about_friend_links
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean).length,
);

const overviewCards = computed(() => [
  {
    label: "评论审核",
    status: "当前策略",
    tone: "info",
    value: reviewModeLabel.value,
    hint: form.comment_review_mode === "manual_all" ? "风险更低" : "效率更高",
  },
  {
    label: "邮件通知",
    status: mailReady.value ? "已就绪" : "待补全",
    tone: mailReady.value ? "success" : "warn",
    value: form.admin_notify_email || "未设置管理员邮箱",
    hint: form.smtp_host || "缺少 SMTP Host",
  },
  {
    label: "LLM",
    status: llmReady.value ? "已配置" : "未配置",
    tone: llmReady.value ? "success" : "neutral",
    value: form.llm_model || "未设置模型",
    hint: form.llm_base_url || "缺少 Base URL",
  },
  {
    label: "语义搜索",
    status: form.semantic_search_enabled ? "已启用" : "已关闭",
    tone: form.semantic_search_enabled ? (semanticReady.value ? "success" : "warn") : "neutral",
    value: form.embedding_model || "未设置 embedding 模型",
    hint: form.semantic_search_enabled ? `超时 ${form.embedding_timeout_ms} ms` : "未启用向量检索",
  },
  {
    label: "Agent Key",
    status: latestRawKey.value ? "新 Key 待保存" : "访问凭证",
    tone: latestRawKey.value ? "warn" : "neutral",
    value: agentKeySummary.value,
    hint: latestRawKey.value ? "新建的原始 key 只展示一次" : "用于 agent 接口访问",
  },
  {
    label: "About 页面",
    status: hasText(form.about_name) ? "已配置" : "待完善",
    tone: hasText(form.about_name) && hasText(form.about_bio) ? "success" : "warn",
    value: form.about_name || "未设置展示名称",
    hint: aboutFriendCount.value > 0 ? `${aboutFriendCount.value} 条友链` : "未填写友链",
  },
]);

async function loadSettingsData() {
  const settings = await apiFetch<AppSettings>("/api/admin/settings", {
    headers: { Authorization: `Bearer ${auth.token}` },
  });
  Object.assign(form, settings);
}

async function saveSettings() {
  saving.value = true;
  try {
    const settings = await apiFetch<AppSettings>(
      "/api/admin/settings",
      jsonRequest("PUT", form, auth.token),
    );
    Object.assign(form, settings);
    Message.success("设置已保存");
  } catch (error) {
    Message.error("保存设置失败");
    console.error(error);
  } finally {
    saving.value = false;
  }
}

async function loadAgentKeys() {
  const response = await apiFetch<{ keys: AgentAPIKey[] }>("/api/admin/agent-keys", {
    headers: { Authorization: `Bearer ${auth.token}` },
  });
  agentKeys.value = response.keys;
}

async function createAgentKey() {
  creatingKey.value = true;
  try {
    const response = await apiFetch<{ key: AgentAPIKey; raw_key: string }>(
      "/api/admin/agent-keys",
      jsonRequest("POST", { name: keyName.value }, auth.token),
    );
    latestRawKey.value = response.raw_key;
    keyName.value = "";
    Message.success("Agent key 已创建");
    await loadAgentKeys();
  } catch (error) {
    Message.error("创建 agent key 失败");
    console.error(error);
  } finally {
    creatingKey.value = false;
  }
}

async function revokeAgentKey(id: number) {
  try {
    await apiFetch<{ ok: boolean }>(`/api/admin/agent-keys/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("Agent key 已吊销");
    await loadAgentKeys();
  } catch (error) {
    Message.error("吊销 agent key 失败");
    console.error(error);
  }
}

function formatDate(raw: string | null) {
  if (!raw) return "未使用";
  const value = new Date(raw);
  if (Number.isNaN(value.getTime())) return raw;
  return value.toLocaleString("zh-CN", { hour12: false });
}

async function loadPage() {
  loading.value = true;
  try {
    await Promise.all([loadSettingsData(), loadAgentKeys()]);
  } catch (error) {
    Message.error("加载设置失败");
    console.error(error);
  } finally {
    loading.value = false;
  }
}

onMounted(loadPage);
</script>

<template>
  <section class="page-stack settings-page">
    <header class="page-header settings-header">
      <div class="settings-heading">
        <h1>系统设置</h1>
        <div class="page-sub">把通知、审核、模型和接口凭证集中维护在同一页</div>
      </div>
      <div class="settings-header-actions">
        <span class="settings-save-hint">保存后立即生效</span>
        <a-button type="primary" :loading="saving" @click="saveSettings">
          <template #icon><IconSave /></template>
          保存设置
        </a-button>
      </div>
    </header>

    <a-spin :loading="loading" style="display: block">
      <div class="settings-overview">
        <article
          v-for="item in overviewCards"
          :key="item.label"
          class="settings-overview-card"
        >
          <div class="settings-overview-top">
            <span class="settings-overview-label">{{ item.label }}</span>
            <span class="settings-pill" :data-tone="item.tone">{{ item.status }}</span>
          </div>
          <strong class="settings-overview-value">{{ item.value }}</strong>
          <span class="settings-overview-hint">{{ item.hint }}</span>
        </article>
      </div>

      <div class="settings-grid">
        <a-card :bordered="true" class="settings-card settings-full">
          <template #title>
            <div class="settings-card-title">
              <strong>通知与邮件</strong>
              <span>管理员收件箱、SMTP 发信和寄件身份</span>
            </div>
          </template>
          <template #extra>
            <span class="settings-pill" :data-tone="mailReady ? 'success' : 'warn'">
              {{ mailReady ? "已就绪" : "待补全" }}
            </span>
          </template>
          <a-form :model="form" layout="vertical" class="settings-form">
            <a-row :gutter="12">
              <a-col :span="12">
                <a-form-item field="admin_notify_email" label="管理员邮箱">
                  <a-input
                    v-model="form.admin_notify_email"
                    placeholder="用于接收评论审核与互动通知"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="smtp_from" label="发件地址">
                  <a-input v-model="form.smtp_from" placeholder="noreply@example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="10">
                <a-form-item field="smtp_host" label="SMTP Host">
                  <a-input v-model="form.smtp_host" placeholder="smtp.example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item field="smtp_port" label="SMTP Port">
                  <a-input v-model="form.smtp_port" placeholder="587" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="smtp_username" label="SMTP Username">
                  <a-input v-model="form.smtp_username" placeholder="bot@example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="smtp_password" label="SMTP Password">
                  <a-input-password v-model="form.smtp_password" placeholder="应用专用密码或 SMTP 密码" />
                </a-form-item>
              </a-col>
            </a-row>
            <div class="settings-note">
              SMTP 留空时系统仍会记录模拟发送日志；配置完整后才会真正发送通知邮件。
            </div>
          </a-form>
        </a-card>

        <a-card :bordered="true" class="settings-card">
          <template #title>
            <div class="settings-card-title">
              <strong>评论审核</strong>
              <span>控制评论进入站点前的审查方式</span>
            </div>
          </template>
          <template #extra>
            <span class="settings-pill" data-tone="info">{{ reviewModeLabel }}</span>
          </template>
          <a-form :model="form" layout="vertical" class="settings-form">
            <a-form-item field="comment_review_mode" label="审核模式">
              <a-radio-group v-model="form.comment_review_mode" type="button" class="settings-mode-group">
                <a-radio value="manual_all">必须人工审核</a-radio>
                <a-radio value="auto_approve_ai_passed">AI 通过后直接放行</a-radio>
              </a-radio-group>
            </a-form-item>
            <div class="settings-note">{{ reviewModeHint }}</div>
          </a-form>
        </a-card>

        <a-card :bordered="true" class="settings-card settings-full">
          <template #title>
            <div class="settings-card-title">
              <strong>AI 与语义搜索</strong>
              <span>审核模型、Embedding 接口和检索开关</span>
            </div>
          </template>
          <div class="settings-cluster-grid">
            <section class="settings-subpanel">
              <div class="settings-subpanel-head">
                <strong>LLM 配置</strong>
                <span class="settings-pill" :data-tone="llmReady ? 'success' : 'neutral'">
                  {{ llmReady ? "已配置" : "未配置" }}
                </span>
              </div>
              <a-form :model="form" layout="vertical" class="settings-form">
                <a-row :gutter="12">
                  <a-col :span="12">
                    <a-form-item field="llm_base_url" label="Base URL">
                      <a-input v-model="form.llm_base_url" placeholder="https://api.openai.com/v1" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item field="llm_model" label="Model">
                      <a-input v-model="form.llm_model" placeholder="gpt-5-mini / 其他模型名" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="24">
                    <a-form-item field="llm_api_key" label="API Key">
                      <a-input-password v-model="form.llm_api_key" placeholder="用于评论审核或生成链路" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="24">
                    <a-form-item field="llm_system_prompt" label="System Prompt">
                      <a-textarea
                        v-model="form.llm_system_prompt"
                        :auto-size="{ minRows: 4, maxRows: 8 }"
                        placeholder="配置默认提示词，用于评论审核或后续文章生成工作流"
                      />
                    </a-form-item>
                  </a-col>
                </a-row>
              </a-form>
            </section>

            <section class="settings-subpanel">
              <div class="settings-switch-row">
                <div class="settings-switch-copy">
                  <strong>语义搜索</strong>
                  <span>搜索页会提供语义召回，异常时自动降级为关键词搜索。</span>
                </div>
                <a-switch v-model="form.semantic_search_enabled" />
              </div>
              <a-form :model="form" layout="vertical" class="settings-form">
                <a-row :gutter="12">
                  <a-col :span="12">
                    <a-form-item field="embedding_base_url" label="Embedding Base URL">
                      <a-input v-model="form.embedding_base_url" placeholder="https://api.openai.com/v1" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item field="embedding_model" label="Embedding Model">
                      <a-input v-model="form.embedding_model" placeholder="text-embedding-3-small / 兼容模型" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="24">
                    <a-form-item field="embedding_api_key" label="Embedding API Key">
                      <a-input-password v-model="form.embedding_api_key" placeholder="用于在线 embedding 接口" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item field="embedding_dimensions" label="Embedding Dimensions">
                      <a-input-number v-model="form.embedding_dimensions" :min="0" :step="1" />
                    </a-form-item>
                    <div class="settings-note">填 0 表示自动检测返回向量维度。</div>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item field="embedding_timeout_ms" label="Timeout (ms)">
                      <a-input-number v-model="form.embedding_timeout_ms" :min="1000" :step="1000" />
                    </a-form-item>
                  </a-col>
                </a-row>
              </a-form>
            </section>
          </div>
        </a-card>

        <a-card :bordered="true" class="settings-card settings-full">
          <template #title>
            <div class="settings-card-title">
              <strong>Agent Key</strong>
              <span>管理 agent 接口访问凭证</span>
            </div>
          </template>
          <template #extra>
            <span class="settings-pill" :data-tone="latestRawKey ? 'warn' : 'neutral'">
              {{ latestRawKey ? "新建未保存" : agentKeySummary }}
            </span>
          </template>

          <div class="agent-key-create">
            <a-input
              v-model="keyName"
              placeholder="可选备注，例如：deploy-agent"
              allow-clear
            />
            <a-button type="primary" :loading="creatingKey" @click="createAgentKey">
              创建 Key
            </a-button>
          </div>

          <div v-if="latestRawKey" class="agent-key-secret">
            <div class="settings-note">新创建的 key 只会展示这一次，请立即保存到安全位置。</div>
            <a-textarea :model-value="latestRawKey" readonly :auto-size="{ minRows: 2, maxRows: 4 }" />
          </div>

          <div v-if="agentKeys.length" class="agent-key-list">
            <article v-for="item in agentKeys" :key="item.id" class="agent-key-item">
              <div class="agent-key-item-head">
                <div>
                  <strong>{{ item.name || "Agent Key" }}</strong>
                  <div class="agent-key-meta">
                    <span>前缀 {{ item.key_prefix }}</span>
                    <span>创建于 {{ formatDate(item.created_at) }}</span>
                  </div>
                  <div class="agent-key-meta">
                    <span>最近使用 {{ formatDate(item.last_used_at) }}</span>
                  </div>
                </div>
                <div class="agent-key-actions">
                  <a-tag v-if="item.revoked_at" color="red">已吊销</a-tag>
                  <a-popconfirm
                    v-else
                    content="吊销后该 key 将不能继续访问 agent 接口，确认？"
                    type="warning"
                    @ok="revokeAgentKey(item.id)"
                  >
                    <a-button size="mini" status="danger">吊销</a-button>
                  </a-popconfirm>
                </div>
              </div>
            </article>
          </div>
          <a-empty v-else description="暂时没有 agent key" :image-size="48" class="settings-empty" />
        </a-card>

        <a-card :bordered="true" class="settings-card settings-full">
          <template #title>
            <div class="settings-card-title">
              <strong>About 页面</strong>
              <span>控制公开 about 页的头像、简介、社交链接、开源统计与友链</span>
            </div>
          </template>
          <template #extra>
            <span class="settings-pill" :data-tone="hasText(form.about_name) && hasText(form.about_bio) ? 'success' : 'warn'">
              {{ hasText(form.about_name) && hasText(form.about_bio) ? "已配置" : "待完善" }}
            </span>
          </template>
          <a-form :model="form" layout="vertical" class="settings-form">
            <a-row :gutter="12">
              <a-col :span="8">
                <a-form-item field="about_name" label="展示名称">
                  <a-input v-model="form.about_name" placeholder="Developer / Snemc" />
                </a-form-item>
              </a-col>
              <a-col :span="16">
                <a-form-item field="about_tagline" label="一句话介绍">
                  <a-input v-model="form.about_tagline" placeholder="构建简洁、优雅的数字体验" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="about_avatar_url" label="头像 URL">
                  <a-input v-model="form.about_avatar_url" placeholder="https://example.com/avatar.jpg" />
                </a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item field="about_email" label="邮箱">
                  <a-input v-model="form.about_email" placeholder="hello@example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item field="about_github_url" label="GitHub URL">
                  <a-input v-model="form.about_github_url" placeholder="https://github.com/..." />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="about_bio" label="简介">
                  <a-textarea
                    v-model="form.about_bio"
                    :auto-size="{ minRows: 4, maxRows: 8 }"
                    placeholder="热爱技术与设计的开发者。专注于前端工程化、用户界面构建与开发者体验优化。"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="about_repo_count" label="仓库数量">
                  <a-input v-model="form.about_repo_count" placeholder="120+" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="about_star_count" label="Stars">
                  <a-input v-model="form.about_star_count" placeholder="3.2k" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="about_fork_count" label="Forks">
                  <a-input v-model="form.about_fork_count" placeholder="450" />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="about_friend_links" label="友链列表">
                  <a-textarea
                    v-model="form.about_friend_links"
                    :auto-size="{ minRows: 4, maxRows: 10 }"
                    placeholder="Alex Chen|全栈开发者 / 技术博主|https://example.com|#667eea,#764ba2"
                  />
                </a-form-item>
                <div class="settings-note">
                  每行一个友链，格式为：名称 | 描述 | 链接 | 渐变色1,渐变色2。
                  颜色可选，缺省时系统会自动分配。
                </div>
              </a-col>
            </a-row>
          </a-form>
        </a-card>
      </div>
    </a-spin>
  </section>
</template>

<style scoped>
.settings-page {
  gap: 16px;
}

.settings-header {
  align-items: flex-start;
  margin-bottom: 0;
}

.settings-heading {
  display: grid;
  gap: 6px;
}

.settings-header-actions {
  display: inline-flex;
  align-items: center;
  gap: 12px;
}

.settings-save-hint {
  color: var(--color-text-3);
  font-size: 12px;
  white-space: nowrap;
}

.settings-overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.settings-overview-card {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 14px 16px;
  border: 1px solid var(--color-neutral-3);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.98));
}

.settings-overview-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.settings-overview-label {
  color: var(--color-text-3);
  font-size: 12px;
  font-weight: 500;
}

.settings-overview-value {
  color: var(--color-text-1);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.35;
}

.settings-overview-hint {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.5;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.settings-full {
  grid-column: 1 / -1;
}

.settings-card-title {
  display: grid;
  gap: 3px;
}

.settings-card-title strong {
  color: var(--color-text-1);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.25;
}

.settings-card-title span {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.45;
}

.settings-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
  background: var(--color-fill-2);
  color: var(--color-text-2);
}

.settings-pill[data-tone="success"] {
  background: rgba(0, 180, 42, 0.12);
  color: #0f8a2f;
}

.settings-pill[data-tone="warn"] {
  background: rgba(255, 125, 0, 0.12);
  color: #c25100;
}

.settings-pill[data-tone="info"] {
  background: rgba(22, 93, 255, 0.12);
  color: rgb(var(--primary-6));
}

.settings-pill[data-tone="neutral"] {
  background: rgba(134, 144, 156, 0.12);
  color: #58667a;
}

.settings-form :deep(.arco-form-item) {
  margin-bottom: 12px;
}

.settings-form :deep(.arco-form-item-label-col) {
  padding-bottom: 4px;
}

.settings-form :deep(.arco-input-number) {
  width: 100%;
}

.settings-form :deep(.arco-textarea-wrapper),
.settings-form :deep(.arco-input-wrapper) {
  border-radius: 12px;
}

.settings-mode-group {
  width: 100%;
}

.settings-mode-group :deep(.arco-radio-group-button) {
  flex: 1;
  text-align: center;
}

.settings-note {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.65;
}

.settings-cluster-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.settings-subpanel {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--color-neutral-3);
  border-radius: 14px;
  background: var(--color-fill-1);
}

.settings-subpanel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.settings-subpanel-head strong {
  color: var(--color-text-1);
  font-size: 13px;
  font-weight: 700;
}

.settings-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 14px;
  border-radius: 14px;
  background: rgba(22, 93, 255, 0.06);
}

.settings-switch-copy {
  display: grid;
  gap: 4px;
}

.settings-switch-copy strong {
  color: var(--color-text-1);
  font-size: 13px;
  font-weight: 700;
}

.settings-switch-copy span {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.55;
}

.agent-key-create {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  margin-bottom: 14px;
}

.agent-key-secret {
  margin-bottom: 16px;
}

.agent-key-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.agent-key-item {
  border: 1px solid var(--color-neutral-3);
  border-radius: 14px;
  padding: 14px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.9), rgba(255, 255, 255, 0.96));
}

.agent-key-item-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: flex-start;
  gap: 12px;
}

.agent-key-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 6px;
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.6;
}

.agent-key-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.settings-empty {
  margin: 12px 0 4px;
}

@media (max-width: 960px) {
  .settings-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .settings-header-actions {
    width: 100%;
    justify-content: space-between;
  }

  .settings-grid {
    grid-template-columns: 1fr;
  }

  .settings-full {
    grid-column: auto;
  }

  .settings-cluster-grid {
    grid-template-columns: 1fr;
  }

  .agent-key-create {
    grid-template-columns: 1fr;
  }

  .agent-key-item-head {
    grid-template-columns: 1fr;
  }
}
</style>
