<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Message } from "@arco-design/web-vue";
import { apiFetch, jsonRequest } from "../api";
import MarkdownEditor from "../components/admin/MarkdownEditor.vue";
import { useAuthStore } from "../stores/auth";
import type { PostDetail, TaxonomyBundle } from "../types";

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const form = reactive({
  id: 0,
  title: "",
  slug: "",
  summary: "",
  markdown: "",
  cover_image: "",
  status: "draft",
  category_name: "Engineering",
  tags: [] as string[],
});
const taxonomies = ref<TaxonomyBundle>({ categories: [], tags: [] });
const saving = ref(false);
const loading = ref(false);
const editorScrollTop = ref(0);

const isEdit = computed(() => route.params.id !== undefined);
const heroCollapsed = computed(() => editorScrollTop.value > 24);

async function loadTaxonomies() {
  taxonomies.value = await apiFetch<TaxonomyBundle>(
    "/api/admin/taxonomies",
    { headers: { Authorization: `Bearer ${auth.token}` } },
  );
}

async function loadPost() {
  if (!isEdit.value) {
    return;
  }
  loading.value = true;
  try {
    const post = await apiFetch<PostDetail>(
      `/api/admin/posts/${route.params.id}`,
      { headers: { Authorization: `Bearer ${auth.token}` } },
    );
    form.id = post.id;
    form.title = post.title;
    form.slug = post.slug;
    form.summary = post.summary;
    form.markdown = post.markdown_source;
    form.cover_image = post.cover_image;
    form.status = post.status;
    form.category_name = post.category_name || "Engineering";
    form.tags = post.tags.map((tag) => tag.name);
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!form.title.trim()) {
    Message.warning("请填写标题");
    return;
  }
  saving.value = true;
  try {
    const method = isEdit.value ? "PUT" : "POST";
    const endpoint = isEdit.value
      ? `/api/admin/posts/${route.params.id}`
      : "/api/admin/posts";
    const response = await apiFetch<PostDetail>(
      endpoint,
      jsonRequest(method, { ...form }, auth.token),
    );
    Message.success(isEdit.value ? "已保存" : "已创建");
    if (!isEdit.value) {
      await router.replace(`/posts/${response.id}`);
    }
  } catch (error) {
    Message.error("保存失败");
    console.error(error);
  } finally {
    saving.value = false;
  }
}

function addTagFromPool(name: string) {
  if (!form.tags.includes(name)) {
    form.tags = [...form.tags, name];
  }
}

function handleEditorScroll(scrollTop: number) {
  editorScrollTop.value = scrollTop;
}

onMounted(async () => {
  await loadTaxonomies();
  await loadPost();
});
</script>

<template>
  <section class="page-stack editor-page">
    <a-spin :loading="loading" style="display: block">
      <div class="editor-shell">
        <section class="editor-main">
          <div class="editor-stage">
            <div class="editor-stage-top" :class="{ 'is-collapsed': heroCollapsed }">
              <div class="editor-stage-hero">
                <a-input
                  v-model="form.title"
                  class="editor-title-input"
                  size="large"
                  placeholder="例如：构建高性能技术博客的渲染链路"
                  allow-clear
                />
                <a-textarea
                  v-model="form.summary"
                  class="editor-summary-input"
                  :auto-size="{ minRows: 2, maxRows: 4 }"
                  placeholder="文章摘要会用于首页卡片、搜索结果和分享海报"
                />
              </div>

              <a-space class="editor-stage-actions" :size="8">
                <a-button @click="router.back()">返回</a-button>
                <a-button type="primary" :loading="saving" @click="save">
                  <template #icon><icon-save /></template>
                  保存
                </a-button>
              </a-space>
            </div>

            <MarkdownEditor
              v-model="form.markdown"
              :token="auth.token"
              @editor-scroll="handleEditorScroll"
            />
          </div>
        </section>

        <aside class="editor-sidebar">
          <a-card :bordered="true">
            <template #title>
              <span style="font-weight: 600">文章属性</span>
            </template>

            <a-form :model="form" layout="vertical">
              <a-form-item field="category_name" label="分类">
                <a-select
                  v-model="form.category_name"
                  placeholder="选择或输入分类"
                  allow-create
                  allow-search
                >
                  <a-option
                    v-for="category in taxonomies.categories"
                    :key="category.id"
                    :value="category.name"
                  >
                    {{ category.name }}
                  </a-option>
                </a-select>
              </a-form-item>

              <a-form-item field="tags" label="标签">
                <a-input-tag
                  v-model="form.tags"
                  placeholder="回车添加，支持多个"
                  allow-clear
                />
              </a-form-item>

              <a-form-item field="status" label="状态">
                <a-select v-model="form.status">
                  <a-option value="draft">草稿</a-option>
                  <a-option value="published">发布</a-option>
                </a-select>
              </a-form-item>

              <a-form-item field="cover_image" label="封面图">
                <a-input
                  v-model="form.cover_image"
                  placeholder="/media/ab/cd/example.webp"
                  allow-clear
                />
              </a-form-item>
            </a-form>
          </a-card>

          <a-card :bordered="true" size="small">
            <template #title>
              <span style="font-weight: 600">编辑器能力</span>
            </template>
            <ul
              style="
                margin: 0;
                padding-left: 18px;
                font-size: 13px;
                color: var(--color-text-2);
                line-height: 1.8;
              "
            >
              <li>结构化标题、列表、引用与表格节点</li>
              <li>代码块单区域编辑，输入与高亮在同一表面完成</li>
              <li>正文仍然保存为 Markdown，可直接复制或导出</li>
              <li>图片继续走当前博客内置图床</li>
            </ul>
          </a-card>

          <a-card :bordered="true" size="small">
            <template #title>
              <span style="font-weight: 600">标签池</span>
            </template>
            <a-space wrap :size="6">
              <a-tag
                v-for="tag in taxonomies.tags"
                :key="tag.id"
                checkable
                :checked="form.tags.includes(tag.name)"
                @check="addTagFromPool(tag.name)"
              >
                {{ tag.name }}
              </a-tag>
            </a-space>
            <a-empty
              v-if="taxonomies.tags.length === 0"
              description="暂无标签"
              :image-size="48"
              style="margin: 8px 0"
            />
          </a-card>

          <a-card :bordered="true" size="small">
            <template #title>
              <span style="font-weight: 600">发布清单</span>
            </template>
            <ul
              style="
                margin: 0;
                padding-left: 18px;
                font-size: 12px;
                color: var(--color-text-3);
                line-height: 1.8;
              "
            >
              <li>标题与 slug 唯一且可读</li>
              <li>摘要不超过 120 字</li>
              <li>至少一个分类与标签</li>
              <li>状态切至「发布」后对外可见</li>
            </ul>
          </a-card>
        </aside>
      </div>
    </a-spin>
  </section>
</template>
