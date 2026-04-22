<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { Message } from "@arco-design/web-vue";
import { apiFetch } from "../api";
import { useAuthStore } from "../stores/auth";
import type { PostSummary } from "../types";

const auth = useAuthStore();
const posts = ref<PostSummary[]>([]);
const loading = ref(false);
const keyword = ref("");
const statusFilter = ref<string>("");

const filteredPosts = computed(() => {
  let list = posts.value;
  if (statusFilter.value) {
    list = list.filter((p) => p.status === statusFilter.value);
  }
  if (keyword.value.trim()) {
    const kw = keyword.value.trim().toLowerCase();
    list = list.filter(
      (p) =>
        p.title.toLowerCase().includes(kw) ||
        p.slug.toLowerCase().includes(kw),
    );
  }
  return list;
});

const columns = [
  { title: "标题", slotName: "title", ellipsis: true, tooltip: true },
  { title: "状态", slotName: "status", width: 90 },
  { title: "分类", dataIndex: "category_name", width: 130, ellipsis: true },
  { title: "标签", slotName: "tags", width: 200 },
  { title: "浏览", dataIndex: "views", width: 70, align: "right" as const },
  { title: "点赞", dataIndex: "likes", width: 70, align: "right" as const },
  { title: "更新", slotName: "updated", width: 140 },
  { title: "操作", slotName: "actions", width: 180, fixed: "right" as const },
];

function formatDate(raw: string) {
  if (!raw) return "-";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

async function loadPosts() {
  loading.value = true;
  try {
    const response = await apiFetch<{ posts: PostSummary[] }>(
      "/api/admin/posts",
      { headers: { Authorization: `Bearer ${auth.token}` } },
    );
    posts.value = response.posts;
  } catch (error) {
    Message.error("加载文章列表失败");
    console.error(error);
  } finally {
    loading.value = false;
  }
}

async function removePost(id: number) {
  try {
    await fetch(`/api/admin/posts/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("已删除");
    await loadPosts();
  } catch {
    Message.error("删除失败");
  }
}

onMounted(loadPosts);
</script>

<template>
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>文章管理</h1>
        <div class="page-sub">维护站点内所有 Markdown 文章</div>
      </div>
      <RouterLink to="/posts/new">
        <a-button type="primary">
          <template #icon><icon-plus /></template>
          新建文章
        </a-button>
      </RouterLink>
    </header>

    <a-card :bordered="true">
      <a-space :size="8" style="margin-bottom: 12px; width: 100%">
        <a-input
          v-model="keyword"
          placeholder="搜索标题或 slug"
          allow-clear
          style="width: 240px"
        >
          <template #prefix><icon-search /></template>
        </a-input>
        <a-select
          v-model="statusFilter"
          placeholder="状态筛选"
          allow-clear
          style="width: 140px"
        >
          <a-option value="published">已发布</a-option>
          <a-option value="draft">草稿</a-option>
        </a-select>
        <a-button type="text" @click="loadPosts">
          <template #icon><icon-refresh /></template>
          刷新
        </a-button>
        <span
          style="margin-left: auto; color: var(--color-text-3); font-size: 12px"
        >
          共 {{ filteredPosts.length }} 篇
        </span>
      </a-space>

      <a-table
        :columns="columns"
        :data="filteredPosts"
        :loading="loading"
        :pagination="{ pageSize: 20, showTotal: true, showPageSize: true }"
        :scroll="{ x: 1100 }"
        row-key="id"
        size="small"
      >
        <template #title="{ record }">
          <div style="display: flex; flex-direction: column; line-height: 1.4">
            <strong style="font-weight: 500">{{ record.title }}</strong>
            <small style="color: var(--color-text-3); font-size: 12px">
              访问 ID: {{ record.slug }}
            </small>
          </div>
        </template>
        <template #status="{ record }">
          <a-tag
            :color="record.status === 'published' ? 'green' : 'gray'"
            size="small"
          >
            {{ record.status === "published" ? "已发布" : "草稿" }}
          </a-tag>
        </template>
        <template #tags="{ record }">
          <a-space :size="4" wrap>
            <a-tag
              v-for="tag in record.tags"
              :key="tag.id"
              size="small"
              color="arcoblue"
            >
              {{ tag.name }}
            </a-tag>
          </a-space>
        </template>
        <template #updated="{ record }">
          <span style="color: var(--color-text-3); font-size: 12px">
            {{ formatDate(record.updated_at) }}
          </span>
        </template>
        <template #actions="{ record }">
          <a-space :size="4">
            <RouterLink :to="`/posts/${record.id}`">
              <a-button type="text" size="mini">
                <template #icon><icon-edit /></template>
                编辑
              </a-button>
            </RouterLink>
            <a
              :href="`/posts/${record.slug}`"
              target="_blank"
              rel="noreferrer"
            >
              <a-button type="text" size="mini">
                <template #icon><icon-eye /></template>
                预览
              </a-button>
            </a>
            <a-popconfirm
              content="确认删除该文章?此操作不可撤销"
              type="warning"
              @ok="removePost(record.id)"
            >
              <a-button type="text" size="mini" status="danger">
                <template #icon><icon-delete /></template>
                删除
              </a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </a-table>
    </a-card>
  </section>
</template>
