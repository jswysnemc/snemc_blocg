<script setup lang="ts">
import { onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import {
  IconClockCircle,
  IconEdit,
  IconEye,
  IconMessage,
  IconPlus,
  IconSearch,
  IconUser,
} from "@arco-design/web-vue/es/icon";
import { apiFetch } from "../api";
import { useAuthStore } from "../stores/auth";
import type { DashboardStats, PostSummary } from "../types";

const auth = useAuthStore();
const stats = ref<DashboardStats | null>(null);
const posts = ref<PostSummary[]>([]);
const loading = ref(false);

const statCards = [
  { key: "published_posts", label: "已发布文章", icon: IconEdit, accent: "rgba(22, 93, 255, 0.08)", color: "#165dff" },
  { key: "draft_posts", label: "草稿数量", icon: IconClockCircle, accent: "rgba(245, 63, 63, 0.08)", color: "#f53f3f" },
  { key: "pending_comments", label: "待审评论", icon: IconMessage, accent: "rgba(255, 125, 0, 0.08)", color: "#ff7d00" },
  { key: "total_views", label: "累计浏览", icon: IconEye, accent: "rgba(0, 180, 42, 0.08)", color: "#00b42a" },
  { key: "active_visitors", label: "活跃访客 / 7 天", icon: IconUser, accent: "rgba(114, 46, 209, 0.08)", color: "#722ed1" },
  { key: "searches_7d", label: "搜索 / 7 天", icon: IconSearch, accent: "rgba(0, 148, 136, 0.08)", color: "#009488" },
] as const;

const tableColumns = [
  { title: "标题", dataIndex: "title", ellipsis: true, tooltip: true },
  { title: "状态", dataIndex: "status", width: 100, slotName: "status" },
  { title: "分类", dataIndex: "category_name", width: 140 },
  { title: "浏览", dataIndex: "views", width: 80, align: "right" as const },
  {
    title: "更新时间",
    dataIndex: "updated_at",
    width: 180,
    slotName: "updated",
  },
];

function formatDate(raw: string) {
  if (!raw) return "-";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

onMounted(async () => {
  loading.value = true;
  try {
    stats.value = await apiFetch<DashboardStats>("/api/admin/dashboard", {
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    const response = await apiFetch<{ posts: PostSummary[] }>(
      "/api/admin/posts",
      { headers: { Authorization: `Bearer ${auth.token}` } },
    );
    posts.value = response.posts.slice(0, 8);
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>仪表盘</h1>
        <div class="page-sub">站点运营指标与最近编辑活动概览</div>
      </div>
      <RouterLink to="/posts/new">
        <a-button type="primary" size="medium">
          <template #icon><icon-plus /></template>
          新建文章
        </a-button>
      </RouterLink>
    </header>

    <div class="stat-grid">
      <article
        v-for="card in statCards"
        :key="card.key"
        class="stat-card"
        :style="{ '--stat-accent': card.accent, '--stat-color': card.color }"
      >
        <span class="stat-icon">
          <component :is="card.icon" />
        </span>
        <span class="stat-label">{{ card.label }}</span>
        <span class="stat-value">{{ stats ? stats[card.key] : "—" }}</span>
      </article>
    </div>

    <a-card :bordered="true" :body-style="{ padding: '0' }">
      <template #title>
        <span style="font-weight: 700; font-size: 15px; color: #0f172a">最近更新</span>
      </template>
      <template #extra>
        <RouterLink to="/posts">
          <a-link>查看全部</a-link>
        </RouterLink>
      </template>
      <a-table
        :columns="tableColumns"
        :data="posts"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 720 }"
        row-key="id"
        size="small"
        :bordered="{ wrapper: false, cell: false }"
      >
        <template #status="{ record }">
          <a-tag
            :color="record.status === 'published' ? 'green' : 'gray'"
            size="small"
            style="border-radius: 999px"
          >
            {{ record.status === "published" ? "已发布" : "草稿" }}
          </a-tag>
        </template>
        <template #updated="{ record }">
          <span style="color: #94a3b8; font-size: 12px">
            {{ formatDate(record.updated_at) }}
          </span>
        </template>
      </a-table>
    </a-card>
  </section>
</template>
