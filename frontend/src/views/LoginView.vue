<script setup lang="ts">
import { reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { Message } from "@arco-design/web-vue";
import { useAuthStore } from "../stores/auth";

const auth = useAuthStore();
const router = useRouter();

const form = reactive({
  username: "admin",
  password: "ChangeMe123!",
});
const loading = ref(false);

async function submit() {
  loading.value = true;
  try {
    await auth.login(form.username, form.password);
    Message.success("登录成功");
    await router.replace("/dashboard");
  } catch {
    Message.error("登录失败,请检查用户名和密码");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-shell">
    <section class="login-card">
      <div class="login-brand">
        <img src="/logo.svg" alt="Logo" class="login-brand-mark" width="44" height="44" />
        <div>
          <h1>Snemc Blog 后台</h1>
          <p class="login-sub">登录后管理文章、评论、分类与标签</p>
        </div>
      </div>
      <a-form :model="form" layout="vertical" auto-label-width @submit="submit">
        <a-form-item field="username" label="用户名">
          <a-input
            v-model="form.username"
            placeholder="请输入用户名"
            allow-clear
            autocomplete="username"
          >
            <template #prefix><icon-user /></template>
          </a-input>
        </a-form-item>
        <a-form-item field="password" label="密码">
          <a-input-password
            v-model="form.password"
            placeholder="请输入密码"
            autocomplete="current-password"
          >
            <template #prefix><icon-lock /></template>
          </a-input-password>
        </a-form-item>
        <a-form-item>
          <a-button
            type="primary"
            html-type="submit"
            :loading="loading"
            long
            size="medium"
          >
            登录
          </a-button>
        </a-form-item>
      </a-form>
    </section>
  </div>
</template>
