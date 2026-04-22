import { defineStore } from "pinia";
import { apiFetch, jsonRequest } from "../api";
import type { AdminUser } from "../types";

const TOKEN_KEY = "snemc-blog-admin-token";
const USER_KEY = "snemc-blog-admin-user";

interface LoginResponse {
  token: string;
  user: AdminUser;
}

export const useAuthStore = defineStore("auth", {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) ?? "",
    user: loadUser(),
    ready: false,
  }),
  getters: {
    isAuthenticated: (state) => Boolean(state.token),
  },
  actions: {
    async login(username: string, password: string) {
      const response = await apiFetch<LoginResponse>("/api/admin/login", jsonRequest("POST", { username, password }));
      this.token = response.token;
      this.user = response.user;
      localStorage.setItem(TOKEN_KEY, response.token);
      localStorage.setItem(USER_KEY, JSON.stringify(response.user));
    },
    async hydrate() {
      if (!this.token) {
        this.ready = true;
        return;
      }
      try {
        const response = await apiFetch<{ user: AdminUser }>("/api/admin/me", {
          headers: {
            Authorization: `Bearer ${this.token}`,
          },
        });
        this.user = response.user;
        localStorage.setItem(USER_KEY, JSON.stringify(response.user));
      } catch {
        this.logout();
      } finally {
        this.ready = true;
      }
    },
    logout() {
      this.token = "";
      this.user = null;
      this.ready = true;
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    },
  },
});

function loadUser(): AdminUser | null {
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw) as AdminUser;
  } catch {
    return null;
  }
}

