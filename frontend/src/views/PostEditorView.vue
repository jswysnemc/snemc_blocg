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

const isEdit = computed(() => route.params.id !== undefined);

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

onMounted(async () => {
  await loadTaxonomies();
  await loadPost();
});
</script>

<template>
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>{{ isEdit ? "编辑文章" : "创建文章" }}</h1>
        <div class="page-sub">
          支持 Markdown、公式、Mermaid 图表与图片上传
        </div>
      </div>
      <a-space :size="8">
        <a-button @click="router.back()">返回</a-button>
        <a-button type="primary" :loading="saving" @click="save">
          <template #icon><icon-save /></template>
          保存
        </a-button>
      </a-space>
    </header>

    <div class="editor-layout">
      <a-card :bordered="true">
        <a-spin :loading="loading" style="display: block">
          <a-form :model="form" layout="vertical" :auto-label-width="true">
            <a-row :gutter="12">
              <a-col :span="16">
                <a-form-item field="title" label="标题">
                  <a-input
                    v-model="form.title"
                    placeholder="例如:构建高性能技术博客的渲染链路"
                    allow-clear
                  />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="访问 ID">
                  <a-input
                    :model-value="form.slug || '保存后自动生成'"
                    readonly
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="summary" label="摘要">
                  <a-textarea
                    v-model="form.summary"
                    :auto-size="{ minRows: 2, maxRows: 4 }"
                    placeholder="文章摘要会用于首页卡片、搜索结果和分享海报"
                    allow-clear
                  />
                </a-form-item>
              </a-col>
              <a-col :span="8">
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
              </a-col>
              <a-col :span="10">
                <a-form-item field="tags" label="标签">
                  <a-input-tag
                    v-model="form.tags"
                    placeholder="回车添加,支持多个"
                    allow-clear
                  />
                </a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item field="status" label="状态">
                  <a-select v-model="form.status">
                    <a-option value="draft">草稿</a-option>
                    <a-option value="published">发布</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item field="cover_image" label="封面图">
                  <a-input
                    v-model="form.cover_image"
                    placeholder="/media/ab/cd/example.webp"
                    allow-clear
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item label="正文" :show-colon="false">
                  <MarkdownEditor v-model="form.markdown" :token="auth.token" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-spin>
      </a-card>

      <aside class="editor-aside">
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
            <li>即时渲染 Markdown、公式与 Mermaid</li>
            <li>拖拽、粘贴截图或远程图片地址自动接入内置图床</li>
            <li>保存时后端生成安全 HTML 与搜索索引</li>
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
  </section>
</template>
