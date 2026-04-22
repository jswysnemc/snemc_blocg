<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Message } from "@arco-design/web-vue";
import { apiFetch, jsonRequest } from "../api";
import { useAuthStore } from "../stores/auth";
import type { CommentNode } from "../types";

const auth = useAuthStore();
const comments = ref<CommentNode[]>([]);
const loading = ref(false);
const statusFilter = ref<string>("");

const filteredComments = computed(() => {
  if (!statusFilter.value) return comments.value;
  return comments.value.filter((c) => c.status === statusFilter.value);
});

const stats = computed(() => ({
  pending: comments.value.filter((c) => c.status === "pending").length,
  approved: comments.value.filter((c) => c.status === "approved").length,
  rejected: comments.value.filter((c) => c.status === "rejected").length,
  total: comments.value.length,
}));

function statusTag(status: string) {
  if (status === "approved") return { color: "green", text: "已通过" };
  if (status === "rejected") return { color: "red", text: "已拒绝" };
  return { color: "orange", text: "待审核" };
}

function formatDate(raw: string) {
  if (!raw) return "-";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString("zh-CN", { hour12: false });
}

async function loadComments() {
  loading.value = true;
  try {
    const response = await apiFetch<{ comments: CommentNode[] }>(
      "/api/admin/comments",
      { headers: { Authorization: `Bearer ${auth.token}` } },
    );
    comments.value = response.comments;
  } catch {
    Message.error("加载评论失败");
  } finally {
    loading.value = false;
  }
}

async function review(id: number, status: string) {
  try {
    await apiFetch<{ ok: boolean }>(
      `/api/admin/comments/${id}/review`,
      jsonRequest("POST", { status }, auth.token),
    );
    Message.success(status === "approved" ? "已通过" : "已拒绝");
    await loadComments();
  } catch {
    Message.error("操作失败");
  }
}

async function rerunAIReview(id: number) {
  try {
    await apiFetch<{ status: string; reason: string }>(
      `/api/admin/comments/${id}/ai-review`,
      jsonRequest("POST", {}, auth.token),
    );
    Message.success("已重新执行 AI 预审占位接口");
    await loadComments();
  } catch {
    Message.error("重新执行 AI 预审失败");
  }
}

async function removeComment(id: number) {
  try {
    await fetch(`/api/admin/comments/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("已删除");
    await loadComments();
  } catch {
    Message.error("删除失败");
  }
}

onMounted(loadComments);
</script>

<template>
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>评论审核</h1>
        <div class="page-sub">评论先待审核入库，AI 预审接口已预留并可手动重跑</div>
      </div>
      <a-button type="text" @click="loadComments">
        <template #icon><icon-refresh /></template>
        刷新
      </a-button>
    </header>

    <div class="stat-grid" style="grid-template-columns: repeat(4, 1fr)">
      <article class="stat-card">
        <span class="stat-label">全部评论</span>
        <span class="stat-value">{{ stats.total }}</span>
      </article>
      <article class="stat-card">
        <span class="stat-label">待审核</span>
        <span class="stat-value" style="color: rgb(var(--orange-6))">
          {{ stats.pending }}
        </span>
      </article>
      <article class="stat-card">
        <span class="stat-label">已通过</span>
        <span class="stat-value" style="color: rgb(var(--green-6))">
          {{ stats.approved }}
        </span>
      </article>
      <article class="stat-card">
        <span class="stat-label">已拒绝</span>
        <span class="stat-value" style="color: rgb(var(--red-6))">
          {{ stats.rejected }}
        </span>
      </article>
    </div>

    <a-card :bordered="true">
      <a-space :size="8" style="margin-bottom: 12px">
        <a-radio-group
          v-model="statusFilter"
          type="button"
          size="small"
        >
          <a-radio value="">全部</a-radio>
          <a-radio value="pending">待审核</a-radio>
          <a-radio value="approved">已通过</a-radio>
          <a-radio value="rejected">已拒绝</a-radio>
        </a-radio-group>
        <span style="color: var(--color-text-3); font-size: 12px">
          共 {{ filteredComments.length }} 条
        </span>
      </a-space>

      <a-spin :loading="loading" style="display: block">
        <div
          v-if="filteredComments.length === 0 && !loading"
          style="padding: 32px 0"
        >
          <a-empty description="没有符合条件的评论" />
        </div>
        <div
          v-else
          style="display: flex; flex-direction: column; gap: 8px"
        >
          <article
            v-for="comment in filteredComments"
            :key="comment.id"
            class="comment-card"
          >
            <div class="comment-card-head">
              <div class="comment-author">
                <strong>{{ comment.author_name || "匿名身份" }}</strong>
                <small>{{ comment.post_title || "未知文章" }}</small>
              </div>
              <a-tag
                :color="statusTag(comment.status).color"
                size="small"
              >
                {{ statusTag(comment.status).text }}
              </a-tag>
            </div>
            <div class="comment-card-body">{{ comment.content }}</div>
            <div class="comment-card-meta">
              <span>
                <icon-email />
                {{ comment.email || "未填写邮箱" }}
              </span>
              <span>
                <icon-robot />
                AI: {{ comment.ai_review_status || "未处理" }}
                <a-tooltip
                  v-if="comment.ai_review_reason"
                  :content="comment.ai_review_reason"
                >
                  <icon-info-circle
                    style="margin-left: 2px; cursor: help"
                  />
                </a-tooltip>
              </span>
              <span>
                <icon-clock-circle />
                {{ formatDate(comment.created_at) }}
              </span>
              <span>
                <icon-notification />
                邮件: {{ comment.notify_status || "queued" }}
                <a-tooltip
                  v-if="comment.notify_error"
                  :content="comment.notify_error"
                >
                  <icon-info-circle style="margin-left: 2px; cursor: help" />
                </a-tooltip>
              </span>
            </div>
            <div class="comment-card-actions">
              <a-button
                size="mini"
                @click="rerunAIReview(comment.id)"
              >
                <template #icon><icon-robot /></template>
                重跑 AI 预审
              </a-button>
              <a-button
                type="primary"
                size="mini"
                :disabled="comment.status === 'approved'"
                @click="review(comment.id, 'approved')"
              >
                <template #icon><icon-check /></template>
                通过
              </a-button>
              <a-button
                size="mini"
                :disabled="comment.status === 'rejected'"
                @click="review(comment.id, 'rejected')"
              >
                <template #icon><icon-close /></template>
                拒绝
              </a-button>
              <a-popconfirm
                content="删除后不可恢复,确认?"
                type="warning"
                @ok="removeComment(comment.id)"
              >
                <a-button type="text" size="mini" status="danger">
                  <template #icon><icon-delete /></template>
                  删除
                </a-button>
              </a-popconfirm>
            </div>
          </article>
        </div>
      </a-spin>
    </a-card>
  </section>
</template>
