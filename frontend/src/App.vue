<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";
import {
  IconApps,
  IconDashboard,
  IconExport,
  IconFile,
  IconLaunch,
  IconMessage,
  IconSettings,
  IconTags,
} from "@arco-design/web-vue/es/icon";
import { useAuthStore } from "./stores/auth";

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const isPublic = computed(() => route.meta.public === true);

const menuItems = [
  { key: "/dashboard", label: "仪表盘", icon: IconDashboard },
  { key: "/posts", label: "文章管理", icon: IconFile },
  { key: "/comments", label: "评论审核", icon: IconMessage },
  { key: "/taxonomies", label: "分类与标签", icon: IconTags },
  { key: "/assets", label: "静态资源", icon: IconApps },
  { key: "/settings", label: "系统设置", icon: IconSettings },
];

const pageTitle = computed(() => {
  const path = route.path;
  if (path.startsWith("/posts/new")) return "创建文章";
  if (path.startsWith("/posts/") && path !== "/posts") return "编辑文章";
  const item = menuItems.find((m) => path.startsWith(m.key));
  return item?.label ?? "后台控制台";
});

const isEditorPage = computed(() =>
  route.path.startsWith("/posts/new") || (route.path.startsWith("/posts/") && route.path !== "/posts"),
);

const selectedKeys = computed(() => {
  const match = menuItems.find((item) => route.path.startsWith(item.key));
  return match ? [match.key] : ["/dashboard"];
});

function handleMenuClick(key: string) {
  if (key !== route.path) {
    router.push(key);
  }
}

async function logout() {
  auth.logout();
  await router.replace("/login");
}

onMounted(async () => {
  await auth.hydrate();
  if (!auth.isAuthenticated && !isPublic.value) {
    await router.replace("/login");
    return;
  }
  if (auth.isAuthenticated && isPublic.value) {
    await router.replace("/dashboard");
  }
});

watch(
  () => auth.isAuthenticated,
  (value) => {
    if (!value && !isPublic.value) {
      router.replace("/login");
    }
  },
);
</script>

<template>
  <a-config-provider :size="'small'">
    <div v-if="!auth.ready" class="admin-loading">
      <div class="admin-loading-spinner"></div>
      <span>加载中</span>
    </div>

    <RouterView v-else-if="isPublic || !auth.isAuthenticated" />

    <div v-else class="admin-shell">
      <aside class="admin-sider">
        <div class="admin-brand">
          <img src="/logo.svg" alt="Logo" class="admin-brand-mark" width="32" height="32" />
          <span class="admin-brand-text">
            <strong>Snemc Blog</strong>
            <small>Admin Console</small>
          </span>
        </div>
        <div class="admin-menu-wrap">
          <a-menu
            :selected-keys="selectedKeys"
            :style="{ background: 'transparent', border: 'none' }"
            @menu-item-click="handleMenuClick"
          >
            <a-menu-item v-for="item in menuItems" :key="item.key">
              <template #icon>
                <component :is="item.icon" />
              </template>
              {{ item.label }}
            </a-menu-item>
          </a-menu>
        </div>
        <div class="admin-user">
          <div class="admin-user-info">
            <div class="admin-user-avatar">
              {{ auth.user?.username?.[0]?.toUpperCase() ?? "A" }}
            </div>
            <div class="admin-user-meta">
              <strong>{{ auth.user?.username ?? "admin" }}</strong>
              <small>Administrator</small>
            </div>
          </div>
          <a-button type="text" size="mini" status="danger" @click="logout">
            <template #icon><IconExport /></template>
          </a-button>
        </div>
      </aside>
      <div class="admin-body">
        <header class="admin-topbar">
          <div class="topbar-left">
            <span class="topbar-title">{{ pageTitle }}</span>
          </div>
          <a-space :size="8">
            <a href="/" target="_blank" rel="noreferrer">
              <a-button type="text" size="small">
                <template #icon><IconLaunch /></template>
                查看站点
              </a-button>
            </a>
          </a-space>
        </header>
        <main class="admin-main" :class="{ 'admin-main-editor': isEditorPage }">
          <RouterView />
        </main>
      </div>
    </div>
  </a-config-provider>
</template>
