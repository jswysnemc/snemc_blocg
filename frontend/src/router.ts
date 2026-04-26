import { createRouter, createWebHashHistory } from "vue-router";

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: "/login",
      name: "login",
      component: () => import("./views/LoginView.vue"),
      meta: { public: true },
    },
    {
      path: "/",
      redirect: "/dashboard",
    },
    {
      path: "/dashboard",
      name: "dashboard",
      component: () => import("./views/DashboardView.vue"),
    },
    {
      path: "/posts",
      name: "posts",
      component: () => import("./views/PostsView.vue"),
    },
    {
      path: "/posts/new",
      name: "post-new",
      component: () => import("./views/PostEditorView.vue"),
    },
    {
      path: "/posts/:id",
      name: "post-edit",
      component: () => import("./views/PostEditorView.vue"),
    },
    {
      path: "/comments",
      name: "comments",
      component: () => import("./views/CommentsView.vue"),
    },
    {
      path: "/taxonomies",
      name: "taxonomies",
      component: () => import("./views/TaxonomyView.vue"),
    },
    {
      path: "/assets",
      name: "assets",
      component: () => import("./views/MediaAssetsView.vue"),
    },
    {
      path: "/settings",
      name: "settings",
      component: () => import("./views/SettingsView.vue"),
    },
  ],
});

export default router;
