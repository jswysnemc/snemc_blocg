<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Message } from "@arco-design/web-vue";
import {
  IconCopy,
  IconDelete,
  IconDownload,
  IconRefresh,
  IconUpload,
} from "@arco-design/web-vue/es/icon";
import { apiFetch, jsonRequest } from "../api";
import { useAuthStore } from "../stores/auth";
import type { StaticSite } from "../types";

type MediaAsset = {
  name: string;
  path: string;
  url: string;
  markdown_url: string;
  content_type: string;
  width: number;
  height: number;
  size: number;
  modified_at: string;
};

type MediaListResponse = {
  assets: MediaAsset[];
};

type MediaUploadResponse = {
  assets?: MediaAsset[];
};

const auth = useAuthStore();
const loading = ref(false);
const uploading = ref(false);
const importing = ref(false);
const creatingStaticSite = ref(false);
const remoteURL = ref("");
const fileInput = ref<HTMLInputElement | null>(null);
const assets = ref<MediaAsset[]>([]);
const staticSites = ref<StaticSite[]>([]);
const staticSiteUploadBusy = ref<Record<number, boolean>>({});
const staticSiteDownloadBusy = ref<Record<number, boolean>>({});
const staticSiteDeleteBusy = ref<Record<number, boolean>>({});
const staticSiteEntryPaths = ref<Record<number, string>>({});

const totalSize = computed(() => assets.value.reduce((sum, item) => sum + item.size, 0));
const readyStaticSiteCount = computed(() => staticSites.value.filter((item) => isStaticSiteReady(item)).length);
const hostedSitesSize = computed(() => staticSites.value.reduce((sum, item) => sum + item.total_size, 0));

async function loadAssets() {
  loading.value = true;
  try {
    const [mediaResponse, staticSitesResponse] = await Promise.all([
      apiFetch<MediaListResponse>("/api/admin/media/assets", {
        headers: { Authorization: `Bearer ${auth.token}` },
      }),
      apiFetch<{ sites: StaticSite[] }>("/api/admin/static-sites", {
        headers: { Authorization: `Bearer ${auth.token}` },
      }),
    ]);
    assets.value = mediaResponse.assets;
    staticSites.value = staticSitesResponse.sites;
  } catch (error) {
    Message.error("加载静态资源失败");
    console.error(error);
  } finally {
    loading.value = false;
  }
}

function selectFiles() {
  fileInput.value?.click();
}

async function uploadFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  if (files.length === 0) {
    return;
  }

  const formData = new FormData();
  files.forEach((file) => formData.append("image", file));
  uploading.value = true;

  try {
    const response = await fetch("/api/admin/media/images", {
      method: "POST",
      headers: { Authorization: `Bearer ${auth.token}` },
      body: formData,
    });
    if (!response.ok) {
      throw new Error(await response.text());
    }
    const payload = await response.json() as MediaUploadResponse;
    Message.success(`已上传 ${payload.assets?.length || files.length} 个资源`);
    await loadAssets();
  } catch (error) {
    Message.error("上传静态资源失败");
    console.error(error);
  } finally {
    uploading.value = false;
    input.value = "";
  }
}

async function importRemoteAsset() {
  const url = remoteURL.value.trim();
  if (!url) {
    Message.warning("请输入远程图片 URL");
    return;
  }

  importing.value = true;
  try {
    await apiFetch("/api/admin/media/import", jsonRequest("POST", { url }, auth.token));
    remoteURL.value = "";
    Message.success("远程资源已导入");
    await loadAssets();
  } catch (error) {
    Message.error("导入远程资源失败");
    console.error(error);
  } finally {
    importing.value = false;
  }
}

async function copyText(value: string, label: string) {
  await navigator.clipboard.writeText(value);
  Message.success(`${label} 已复制`);
}

async function deleteAsset(asset: MediaAsset) {
  try {
    await apiFetch(`/api/admin/media/assets?path=${encodeURIComponent(asset.path)}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    Message.success("资源已删除");
    await loadAssets();
  } catch (error) {
    Message.error("删除资源失败");
    console.error(error);
  }
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(2)} MB`;
}

function formatDate(raw: string) {
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function dimensions(asset: MediaAsset) {
  if (!asset.width || !asset.height) return "未知尺寸";
  return `${asset.width} x ${asset.height}`;
}

function isStaticSiteReady(site: StaticSite) {
  return Boolean(site.entry_path && site.file_count > 0);
}

function staticSiteModeLabel(site: StaticSite) {
  if (site.storage_mode === "single_file") return "单文件";
  if (site.storage_mode === "directory") return "目录";
  return "未上传";
}

function staticSiteURL(site: StaticSite) {
  if (typeof window === "undefined") {
    return `/h/${site.route_id}/`;
  }
  return new URL(`/h/${site.route_id}/`, window.location.origin).toString();
}

function upsertStaticSite(site: StaticSite) {
  const index = staticSites.value.findIndex((item) => item.id === site.id);
  if (index >= 0) {
    staticSites.value[index] = site;
    return;
  }
  staticSites.value.unshift(site);
}

async function createStaticSite() {
  creatingStaticSite.value = true;
  try {
    const response = await apiFetch<{ site: StaticSite }>(
      "/api/admin/static-sites",
      { method: "POST", headers: { Authorization: `Bearer ${auth.token}` } },
    );
    upsertStaticSite(response.site);
    Message.success("托管网页路由已创建");
  } catch (error) {
    Message.error("创建托管网页失败");
    console.error(error);
  } finally {
    creatingStaticSite.value = false;
  }
}

function triggerStaticSiteUpload(kind: "file" | "directory", id: number) {
  const input = document.getElementById(`static-site-${kind}-${id}`) as HTMLInputElement | null;
  input?.click();
}

function uploadPathForFile(file: File, kind: "file" | "directory") {
  const webkitPath = (file as File & { webkitRelativePath?: string }).webkitRelativePath;
  if (kind === "directory" && webkitPath) return webkitPath;
  return file.name;
}

function inferDownloadName(files: File[], kind: "file" | "directory") {
  if (files.length === 0) return "static-site";
  const firstPath = uploadPathForFile(files[0], kind);
  if (kind === "directory" && firstPath.includes("/")) {
    return firstPath.split("/")[0] || "static-site";
  }
  return files[0].name || "static-site";
}

async function readResponseError(response: Response) {
  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    const payload = (await response.json().catch(() => null)) as { error?: string; msg?: string } | null;
    return payload?.error || payload?.msg || `Request failed: ${response.status}`;
  }
  const text = await response.text().catch(() => "");
  return text || `Request failed: ${response.status}`;
}

function parseDownloadFilename(header: string | null, fallback: string) {
  if (!header) return fallback;
  const utf8Match = header.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1]);
    } catch {
      return utf8Match[1];
    }
  }
  const plainMatch = header.match(/filename="?([^"]+)"?/i);
  return plainMatch?.[1] || fallback;
}

async function uploadStaticSite(site: StaticSite, files: File[], kind: "file" | "directory") {
  if (files.length === 0) return;

  const formData = new FormData();
  files.forEach((file) => {
    formData.append("files", file);
    formData.append("paths", uploadPathForFile(file, kind));
  });

  const entryPath = staticSiteEntryPaths.value[site.id]?.trim();
  if (entryPath && kind === "directory") {
    formData.append("entry_path", entryPath);
  }
  formData.append("download_name", inferDownloadName(files, kind));

  staticSiteUploadBusy.value[site.id] = true;
  try {
    const response = await fetch(`/api/admin/static-sites/${site.id}/upload`, {
      method: "POST",
      headers: { Authorization: `Bearer ${auth.token}` },
      body: formData,
    });
    if (!response.ok) {
      throw new Error(await readResponseError(response));
    }
    const payload = await response.json() as { site: StaticSite };
    upsertStaticSite(payload.site);
    Message.success("托管网页已更新");
  } catch (error) {
    Message.error(error instanceof Error ? error.message : "上传托管网页失败");
    console.error(error);
  } finally {
    staticSiteUploadBusy.value[site.id] = false;
  }
}

async function handleStaticSiteUploadChange(site: StaticSite, kind: "file" | "directory", event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  await uploadStaticSite(site, files, kind);
  input.value = "";
}

async function downloadStaticSite(site: StaticSite) {
  staticSiteDownloadBusy.value[site.id] = true;
  try {
    const response = await fetch(`/api/admin/static-sites/${site.id}/download`, {
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    if (!response.ok) {
      throw new Error(await readResponseError(response));
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    const fallback =
      site.storage_mode === "single_file"
        ? site.download_name || `${site.route_id}.html`
        : `${site.download_name || site.route_id}.zip`;
    link.href = url;
    link.download = parseDownloadFilename(response.headers.get("Content-Disposition"), fallback);
    document.body.append(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  } catch (error) {
    Message.error("下载托管网页失败");
    console.error(error);
  } finally {
    staticSiteDownloadBusy.value[site.id] = false;
  }
}

async function deleteStaticSite(site: StaticSite) {
  staticSiteDeleteBusy.value[site.id] = true;
  try {
    await apiFetch(`/api/admin/static-sites/${site.id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    staticSites.value = staticSites.value.filter((item) => item.id !== site.id);
    Message.success("托管网页已删除");
  } catch (error) {
    Message.error("删除托管网页失败");
    console.error(error);
  } finally {
    staticSiteDeleteBusy.value[site.id] = false;
  }
}

onMounted(loadAssets);
</script>

<template>
  <section class="page-stack media-page">
    <header class="page-header media-header">
      <div>
        <h1>静态资源</h1>
        <div class="page-sub">管理文章媒体和 /h 路由下的托管网页</div>
      </div>
      <div class="media-header-actions">
        <a-button :loading="loading" @click="loadAssets">
          <template #icon><IconRefresh /></template>
          刷新
        </a-button>
        <a-button type="primary" :loading="uploading" @click="selectFiles">
          <template #icon><IconUpload /></template>
          上传图片
        </a-button>
        <input
          ref="fileInput"
          class="media-file-input"
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          multiple
          @change="uploadFiles"
        />
      </div>
    </header>

    <div class="media-overview">
      <article class="media-stat-card">
        <span>资源数量</span>
        <strong>{{ assets.length }}</strong>
        <small>当前媒体目录中的图片文件</small>
      </article>
      <article class="media-stat-card">
        <span>托管网页</span>
        <strong>{{ readyStaticSiteCount }} / {{ staticSites.length }}</strong>
        <small>/h 路由下可访问的独立网页</small>
      </article>
      <article class="media-stat-card">
        <span>总占用</span>
        <strong>{{ formatBytes(totalSize + hostedSitesSize) }}</strong>
        <small>媒体资源和托管网页合计</small>
      </article>
    </div>

    <a-tabs default-active-key="media" class="media-tabs">
      <a-tab-pane key="media" title="媒体资源">
        <a-card :bordered="true" class="media-import-card">
          <template #title>远程导入</template>
          <div class="media-import-row">
            <a-input
              v-model="remoteURL"
              placeholder="https://example.com/image.webp"
              allow-clear
              @press-enter="importRemoteAsset"
            />
            <a-button type="primary" :loading="importing" @click="importRemoteAsset">
              <template #icon><IconDownload /></template>
              导入
            </a-button>
          </div>
          <p>导入后会转存到站点媒体目录，文章中建议使用生成的 Markdown URL。</p>
        </a-card>

        <a-spin :loading="loading" style="display: block">
          <div v-if="assets.length" class="media-grid">
            <article v-for="asset in assets" :key="asset.path" class="media-card">
              <a class="media-preview" :href="asset.url" target="_blank" rel="noreferrer">
                <img :src="asset.url" :alt="asset.name" loading="lazy" />
              </a>
              <div class="media-card-body">
                <div class="media-card-title">
                  <strong :title="asset.name">{{ asset.name }}</strong>
                  <span>{{ formatBytes(asset.size) }}</span>
                </div>
                <div class="media-meta">
                  <span>{{ dimensions(asset) }}</span>
                  <span>{{ asset.content_type }}</span>
                  <span>{{ formatDate(asset.modified_at) }}</span>
                </div>
                <div class="media-path" :title="asset.markdown_url">{{ asset.markdown_url }}</div>
                <div class="media-actions">
                  <a-button size="mini" @click="copyText(asset.markdown_url, 'Markdown URL')">
                    <template #icon><IconCopy /></template>
                    Markdown
                  </a-button>
                  <a-button size="mini" @click="copyText(asset.url, '资源 URL')">
                    <template #icon><IconCopy /></template>
                    URL
                  </a-button>
                  <a-popconfirm
                    content="删除后已引用该资源的文章图片会失效，确认删除？"
                    type="warning"
                    @ok="deleteAsset(asset)"
                  >
                    <a-button size="mini" status="danger">
                      <template #icon><IconDelete /></template>
                      删除
                    </a-button>
                  </a-popconfirm>
                </div>
              </div>
            </article>
          </div>
          <a-empty v-else description="暂无媒体资源" />
        </a-spin>
      </a-tab-pane>

      <a-tab-pane key="hosting" title="托管网页">
        <a-card :bordered="true" class="media-import-card">
          <template #title>网页托管</template>
          <div class="media-import-row">
            <p>创建一个 `/h/{route}/` 路由后，可以上传单个 HTML 文件，或上传包含 HTML、CSS、JS、图片的完整目录。</p>
            <a-button type="primary" :loading="creatingStaticSite" @click="createStaticSite">
              创建路由
            </a-button>
          </div>
        </a-card>

        <a-spin :loading="loading" style="display: block">
          <div v-if="staticSites.length" class="hosted-site-list">
            <article v-for="site in staticSites" :key="site.id" class="hosted-site-item">
              <div class="hosted-site-head">
                <div class="hosted-site-title">
                  <strong>/h/{{ site.route_id }}/</strong>
                  <span class="static-asset-source">{{ staticSiteModeLabel(site) }}</span>
                  <span class="static-asset-source" :data-tone="isStaticSiteReady(site) ? 'success' : 'warn'">
                    {{ isStaticSiteReady(site) ? "已就绪" : "待上传" }}
                  </span>
                </div>
                <div class="media-actions">
                  <a-button size="mini" :disabled="!isStaticSiteReady(site)" @click="copyText(staticSiteURL(site), '托管网页地址')">
                    <template #icon><IconCopy /></template>
                    复制地址
                  </a-button>
                  <a-button size="mini" :href="staticSiteURL(site)" target="_blank" :disabled="!isStaticSiteReady(site)">
                    打开
                  </a-button>
                  <a-button
                    size="mini"
                    :disabled="!isStaticSiteReady(site)"
                    :loading="staticSiteDownloadBusy[site.id]"
                    @click="downloadStaticSite(site)"
                  >
                    下载
                  </a-button>
                  <a-popconfirm
                    content="删除后 /h 路由和已上传文件都会移除，确认？"
                    type="warning"
                    @ok="deleteStaticSite(site)"
                  >
                    <a-button size="mini" status="danger" :loading="staticSiteDeleteBusy[site.id]">
                      <template #icon><IconDelete /></template>
                      删除
                    </a-button>
                  </a-popconfirm>
                </div>
              </div>

              <div class="hosted-site-meta">
                <span>入口 {{ site.entry_path || "未设置" }}</span>
                <span>文件 {{ site.file_count }}</span>
                <span>大小 {{ formatBytes(site.total_size) }}</span>
                <span>更新 {{ formatDate(site.updated_at) }}</span>
              </div>

              <a v-if="isStaticSiteReady(site)" class="media-path hosted-site-url" :href="staticSiteURL(site)" target="_blank" rel="noreferrer">
                {{ staticSiteURL(site) }}
              </a>
              <div v-else class="media-path hosted-site-url">上传 HTML 或目录后自动启用该路由</div>

              <div class="hosted-site-upload">
                <a-input
                  v-model="staticSiteEntryPaths[site.id]"
                  placeholder="可选入口文件，例如 index.html 或 pages/home.html"
                  allow-clear
                />
                <a-button :loading="staticSiteUploadBusy[site.id]" @click="triggerStaticSiteUpload('file', site.id)">
                  上传 HTML
                </a-button>
                <a-button :loading="staticSiteUploadBusy[site.id]" @click="triggerStaticSiteUpload('directory', site.id)">
                  上传目录
                </a-button>
                <input
                  :id="`static-site-file-${site.id}`"
                  class="media-file-input"
                  type="file"
                  accept=".html,.htm,text/html"
                  @change="handleStaticSiteUploadChange(site, 'file', $event)"
                />
                <input
                  :id="`static-site-directory-${site.id}`"
                  class="media-file-input"
                  type="file"
                  multiple
                  webkitdirectory
                  @change="handleStaticSiteUploadChange(site, 'directory', $event)"
                />
              </div>
            </article>
          </div>
          <a-empty v-else description="暂无托管网页路由" />
        </a-spin>
      </a-tab-pane>
    </a-tabs>
  </section>
</template>

<style scoped>
.media-page {
  gap: 16px;
}

.media-header {
  align-items: flex-start;
  margin-bottom: 0;
}

.media-header-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.media-file-input {
  display: none;
}

.media-overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.media-stat-card {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 16px;
  border: 1px solid var(--color-neutral-3);
  border-radius: 18px;
  background:
    linear-gradient(135deg, rgba(22, 93, 255, 0.08), transparent 34%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.98));
}

.media-stat-card span,
.media-stat-card small {
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.5;
}

.media-stat-card strong {
  overflow: hidden;
  color: var(--color-text-1);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-import-card :deep(.arco-card-header) {
  border-bottom: 1px solid var(--color-neutral-3);
}

.media-import-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
}

.media-import-card p {
  margin: 10px 0 0;
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.65;
}

.media-tabs {
  display: block;
}

.media-tabs :deep(.arco-tabs-content) {
  padding-top: 8px;
}

.media-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 14px;
}

.media-card {
  overflow: hidden;
  border: 1px solid var(--color-neutral-3);
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.05);
}

.media-preview {
  display: grid;
  height: 172px;
  place-items: center;
  border-bottom: 1px solid var(--color-neutral-3);
  background:
    linear-gradient(45deg, rgba(148, 163, 184, 0.12) 25%, transparent 25%),
    linear-gradient(-45deg, rgba(148, 163, 184, 0.12) 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, rgba(148, 163, 184, 0.12) 75%),
    linear-gradient(-45deg, transparent 75%, rgba(148, 163, 184, 0.12) 75%);
  background-position: 0 0, 0 8px, 8px -8px, -8px 0;
  background-size: 16px 16px;
}

.media-preview img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.media-card-body {
  display: grid;
  gap: 10px;
  padding: 14px;
}

.media-card-title {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
}

.media-card-title strong {
  overflow: hidden;
  color: var(--color-text-1);
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-card-title span,
.media-meta,
.media-path {
  color: var(--color-text-3);
  font-size: 12px;
}

.media-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  line-height: 1.55;
}

.media-path {
  overflow: hidden;
  padding: 7px 9px;
  border-radius: 10px;
  background: var(--color-fill-2);
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.static-asset-source {
  width: fit-content;
  padding: 3px 8px;
  border-radius: 999px;
  background: rgba(22, 93, 255, 0.1);
  color: rgb(var(--primary-6));
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
}

.static-asset-source[data-tone="success"] {
  background: rgba(0, 180, 42, 0.12);
  color: #0f8a2f;
}

.static-asset-source[data-tone="warn"] {
  background: rgba(255, 125, 0, 0.12);
  color: #c25100;
}

.hosted-site-list {
  display: grid;
  gap: 12px;
}

.hosted-site-item {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--color-neutral-3);
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.04);
}

.hosted-site-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
}

.hosted-site-title {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.hosted-site-title strong {
  color: var(--color-text-1);
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 14px;
  font-weight: 700;
}

.hosted-site-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  color: var(--color-text-3);
  font-size: 12px;
  line-height: 1.6;
}

.hosted-site-url {
  display: block;
}

.hosted-site-upload {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 10px;
  align-items: center;
}

@media (max-width: 960px) {
  .media-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .media-header-actions {
    justify-content: flex-start;
  }

  .media-overview {
    grid-template-columns: 1fr;
  }

  .media-import-row {
    grid-template-columns: 1fr;
  }

  .hosted-site-head,
  .hosted-site-upload {
    grid-template-columns: 1fr;
  }
}
</style>
