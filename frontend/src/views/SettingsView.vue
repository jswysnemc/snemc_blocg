<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
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
});

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
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>系统设置</h1>
        <div class="page-sub">维护邮件、评论审核策略、LLM、语义搜索和 agent key</div>
      </div>
      <a-button type="primary" :loading="saving" @click="saveSettings">
        <template #icon><IconSave /></template>
        保存设置
      </a-button>
    </header>

    <a-spin :loading="loading" style="display: block">
      <div class="settings-grid">
        <a-card :bordered="true">
          <template #title>
            <span style="font-weight: 600">邮件发送配置</span>
          </template>
          <a-form :model="form" layout="vertical">
            <a-row :gutter="12">
              <a-col :span="12">
                <a-form-item field="smtp_host" label="SMTP Host">
                  <a-input v-model="form.smtp_host" placeholder="smtp.example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="smtp_port" label="SMTP Port">
                  <a-input v-model="form.smtp_port" placeholder="587" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="smtp_username" label="SMTP Username">
                  <a-input v-model="form.smtp_username" placeholder="bot@example.com" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="smtp_password" label="SMTP Password">
                  <a-input-password v-model="form.smtp_password" placeholder="应用专用密码或 SMTP 密码" />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="smtp_from" label="From Address">
                  <a-input v-model="form.smtp_from" placeholder="noreply@example.com" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-card>

        <a-card :bordered="true">
          <template #title>
            <span style="font-weight: 600">评论审核策略</span>
          </template>
          <a-form :model="form" layout="vertical">
            <a-form-item field="comment_review_mode" label="审核模式">
              <a-radio-group v-model="form.comment_review_mode" type="button">
                <a-radio value="manual_all">必须人工审核</a-radio>
                <a-radio value="auto_approve_ai_passed">AI 通过后直接放行</a-radio>
              </a-radio-group>
            </a-form-item>
            <div class="settings-note">
              自动放行模式下，只有 AI 明确判定为通过的评论会直接展示，其余评论仍进入人工审核。
            </div>
          </a-form>
        </a-card>

        <a-card :bordered="true">
          <template #title>
            <span style="font-weight: 600">通知收件箱</span>
          </template>
          <a-form :model="form" layout="vertical">
            <a-form-item field="admin_notify_email" label="管理员邮箱">
              <a-input
                v-model="form.admin_notify_email"
                placeholder="用于接收评论数据和互动通知"
              />
            </a-form-item>
            <div class="settings-note">
              评论提交后，后端会把审核数据发送到这个邮箱。SMTP 配置为空时会走模拟发送日志。
            </div>
          </a-form>
        </a-card>

        <a-card :bordered="true" class="settings-full">
          <template #title>
            <span style="font-weight: 600">LLM 配置</span>
          </template>
          <a-form :model="form" layout="vertical">
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
                  <a-input-password v-model="form.llm_api_key" placeholder="用于后续接入 AI 审核或生成能力" />
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
        </a-card>

        <a-card :bordered="true" class="settings-full">
          <template #title>
            <span style="font-weight: 600">语义搜索配置</span>
          </template>
          <a-form :model="form" layout="vertical">
            <a-row :gutter="12">
              <a-col :span="24">
                <a-form-item field="semantic_search_enabled" label="启用语义搜索">
                  <a-switch v-model="form.semantic_search_enabled" />
                </a-form-item>
                <div class="settings-note">
                  启用后，搜索页会提供“语义搜索”入口。服务异常时会自动降级到关键词搜索。
                </div>
              </a-col>
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
        </a-card>

        <a-card :bordered="true" class="settings-full">
          <template #title>
            <span style="font-weight: 600">Agent Key</span>
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
            <div class="settings-note">新创建的 key 只会展示这一次。</div>
            <a-textarea :model-value="latestRawKey" readonly :auto-size="{ minRows: 2, maxRows: 4 }" />
          </div>

          <div v-if="agentKeys.length" class="agent-key-list">
            <article v-for="item in agentKeys" :key="item.id" class="agent-key-item">
              <div class="agent-key-item-head">
                <div>
                  <strong>{{ item.name || "Agent Key" }}</strong>
                  <div class="agent-key-meta">
                    前缀 {{ item.key_prefix }} · 创建于 {{ formatDate(item.created_at) }}
                  </div>
                  <div class="agent-key-meta">
                    最近使用 {{ formatDate(item.last_used_at) }}
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
          <a-empty v-else description="暂时没有 agent key" :image-size="48" />
        </a-card>
      </div>
    </a-spin>
  </section>
</template>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.settings-full {
  grid-column: 1 / -1;
}

.settings-note {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.7;
}

.agent-key-create {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  margin-bottom: 12px;
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
  border-radius: 12px;
  padding: 14px 16px;
  background: var(--color-fill-1);
}

.agent-key-item-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.agent-key-meta {
  margin-top: 4px;
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.7;
}

.agent-key-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 960px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }

  .settings-full {
    grid-column: auto;
  }

  .agent-key-create {
    grid-template-columns: 1fr;
  }

  .agent-key-item-head {
    flex-direction: column;
  }
}
</style>
