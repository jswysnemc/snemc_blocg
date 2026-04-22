<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { Message } from "@arco-design/web-vue";
import { apiFetch, jsonRequest } from "../api";
import { useAuthStore } from "../stores/auth";
import type { TaxonomyBundle } from "../types";

const auth = useAuthStore();
const taxonomies = ref<TaxonomyBundle>({ categories: [], tags: [] });
const loading = ref(false);

const categoryForm = reactive({
  name: "",
  description: "",
});
const tagForm = reactive({
  name: "",
});

async function loadTaxonomies() {
  loading.value = true;
  try {
    taxonomies.value = await apiFetch<TaxonomyBundle>(
      "/api/admin/taxonomies",
      { headers: { Authorization: `Bearer ${auth.token}` } },
    );
  } finally {
    loading.value = false;
  }
}

async function saveCategory() {
  if (!categoryForm.name.trim()) {
    Message.warning("请填写分类名称");
    return;
  }
  try {
    await apiFetch(
      "/api/admin/categories",
      jsonRequest(
        "POST",
        { name: categoryForm.name, description: categoryForm.description },
        auth.token,
      ),
    );
    Message.success("分类已保存");
    categoryForm.name = "";
    categoryForm.description = "";
    await loadTaxonomies();
  } catch {
    Message.error("保存失败");
  }
}

async function saveTag() {
  if (!tagForm.name.trim()) {
    Message.warning("请填写标签名称");
    return;
  }
  try {
    await apiFetch(
      "/api/admin/tags",
      jsonRequest("POST", { name: tagForm.name }, auth.token),
    );
    Message.success("标签已保存");
    tagForm.name = "";
    await loadTaxonomies();
  } catch {
    Message.error("保存失败");
  }
}

async function removeCategory(id: number) {
  try {
    await fetch(`/api/admin/categories/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("已删除");
    await loadTaxonomies();
  } catch {
    Message.error("删除失败");
  }
}

async function removeTag(id: number) {
  try {
    await fetch(`/api/admin/tags/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("已删除");
    await loadTaxonomies();
  } catch {
    Message.error("删除失败");
  }
}

onMounted(loadTaxonomies);
</script>

<template>
  <section class="page-stack">
    <header class="page-header">
      <div>
        <h1>分类与标签</h1>
        <div class="page-sub">统一维护站点分类树与标签池</div>
      </div>
    </header>

    <div class="taxonomy-grid">
      <!-- 分类 -->
      <a-card :bordered="true">
        <template #title>
          <a-space :size="6">
            <icon-folder />
            <span style="font-weight: 600">分类</span>
            <a-tag size="small" color="gray">
              {{ taxonomies.categories.length }}
            </a-tag>
          </a-space>
        </template>

        <a-form :model="categoryForm" layout="vertical">
          <a-form-item field="name" label="分类名称">
            <a-input
              v-model="categoryForm.name"
              placeholder="例如:Engineering"
              allow-clear
            />
          </a-form-item>
          <a-form-item field="description" label="分类描述">
            <a-textarea
              v-model="categoryForm.description"
              :auto-size="{ minRows: 2, maxRows: 4 }"
              placeholder="分类说明,可选"
              allow-clear
            />
          </a-form-item>
          <a-button type="primary" @click="saveCategory">
            <template #icon><icon-plus /></template>
            添加分类
          </a-button>
        </a-form>

        <a-divider :margin="12" />

        <a-spin :loading="loading" style="display: block">
          <div class="taxonomy-list">
            <article
              v-for="category in taxonomies.categories"
              :key="category.id"
              class="taxonomy-item"
            >
              <div>
                <span class="taxonomy-name">{{ category.name }}</span>
                <span class="taxonomy-slug">/ {{ category.slug }}</span>
              </div>
              <a-popconfirm
                content="删除分类不影响已有文章,确认?"
                type="warning"
                @ok="removeCategory(category.id)"
              >
                <a-button type="text" size="mini" status="danger">
                  <template #icon><icon-delete /></template>
                </a-button>
              </a-popconfirm>
            </article>
            <a-empty
              v-if="taxonomies.categories.length === 0"
              description="暂无分类"
              :image-size="64"
            />
          </div>
        </a-spin>
      </a-card>

      <!-- 标签 -->
      <a-card :bordered="true">
        <template #title>
          <a-space :size="6">
            <icon-tag />
            <span style="font-weight: 600">标签</span>
            <a-tag size="small" color="gray">
              {{ taxonomies.tags.length }}
            </a-tag>
          </a-space>
        </template>

        <a-form :model="tagForm" layout="vertical">
          <a-form-item field="name" label="标签名称">
            <a-input
              v-model="tagForm.name"
              placeholder="例如:Vue3"
              allow-clear
              @press-enter="saveTag"
            />
          </a-form-item>
          <a-button type="primary" @click="saveTag">
            <template #icon><icon-plus /></template>
            添加标签
          </a-button>
        </a-form>

        <a-divider :margin="12" />

        <a-spin :loading="loading" style="display: block">
          <a-space wrap :size="6">
            <a-tag
              v-for="tag in taxonomies.tags"
              :key="tag.id"
              color="arcoblue"
              closable
              @close="removeTag(tag.id)"
            >
              {{ tag.name }}
            </a-tag>
          </a-space>
          <a-empty
            v-if="taxonomies.tags.length === 0"
            description="暂无标签"
            :image-size="64"
          />
        </a-spin>
      </a-card>
    </div>
  </section>
</template>
