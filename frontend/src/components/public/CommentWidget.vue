<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from "vue";
import {
  Avatar,
  Button,
  Empty,
  Input,
  Message,
} from "@arco-design/web-vue";
import { IconSend } from "@arco-design/web-vue/es/icon";
import type { CommentNode } from "../../types";
import CommentItem from "./CommentItem.vue";

const VISITOR_NAME_KEY = "snemc-blog-visitor-name";
const VISITOR_EMAIL_KEY = "snemc-blog-visitor-email";

const props = defineProps<{
  slug: string;
}>();

const comments = ref<CommentNode[]>([]);
const form = reactive({
  email: "",
  content: "",
});
const parentId = ref<number | null>(null);
const loading = ref(false);
const emailEditing = ref(false);
const textareaRef = ref<HTMLTextAreaElement | null>(null);
const mentionIndex = ref(0);
const mentionQuery = ref("");
const mentionStart = ref<number | null>(null);

const replyHint = computed(() => {
  if (!parentId.value) return "";
  const findTarget = (list: CommentNode[]): CommentNode | undefined => {
    for (const item of list) {
      if (item.id === parentId.value) return item;
      if (item.replies?.length) {
        const nested = findTarget(item.replies);
        if (nested) return nested;
      }
    }
    return undefined;
  };
  const target = findTarget(comments.value);
  return target ? `正在回复 @${target.author_name || "匿名身份"}` : "";
});

const identityName = computed(() => localStorage.getItem(VISITOR_NAME_KEY) || "匿名身份");
const identityInitial = computed(() => identityName.value.slice(0, 1).toUpperCase());
const showEmailInput = computed(() => emailEditing.value || !form.email.trim());
const maskedEmail = computed(() => maskEmail(form.email));
const mentionableNames = computed(() => {
  const names = new Set<string>();
  const walk = (items: CommentNode[]) => {
    items.forEach((item) => {
      const name = (item.author_name || "").trim();
      if (name) {
        names.add(name);
      }
      if (item.replies?.length) {
        walk(item.replies);
      }
    });
  };
  walk(comments.value);
  names.delete(identityName.value);
  return Array.from(names).slice(0, 12);
});
const filteredMentionNames = computed(() => {
  if (mentionStart.value === null) {
    return [];
  }
  const query = mentionQuery.value.trim().toLowerCase();
  if (!query) {
    return mentionableNames.value.slice(0, 6);
  }
  return mentionableNames.value
    .filter((name) => name.toLowerCase().includes(query))
    .slice(0, 6);
});
const mentionPopupVisible = computed(() => mentionStart.value !== null && filteredMentionNames.value.length > 0);

async function loadComments() {
  const response = await fetch(`/api/posts/${props.slug}/comments`);
  const data = (await response.json()) as { comments: CommentNode[] };
  comments.value = data.comments;
}

async function submit() {
  if (!form.content.trim()) {
    Message.warning("请输入评论内容");
    return;
  }
  const contactEmail = form.email.trim();
  loading.value = true;
  try {
    const response = await fetch(`/api/posts/${props.slug}/comments`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        parent_id: parentId.value,
        author_name: identityName.value,
        email: contactEmail,
        content: form.content,
      }),
    });
    const data = (await response.json()) as {
      message?: string;
      error?: string;
    };
    if (!response.ok) {
      Message.error(data.error ?? "评论提交失败");
      return;
    }
    if (contactEmail) {
      localStorage.setItem(VISITOR_EMAIL_KEY, contactEmail);
      emailEditing.value = false;
    } else {
      localStorage.removeItem(VISITOR_EMAIL_KEY);
    }
    Message.success(data.message ?? "评论已提交,等待审核");
    form.content = "";
    parentId.value = null;
    await loadComments();
  } finally {
    loading.value = false;
  }
}

function insertMention(name: string) {
  const textarea = textareaRef.value;
  const mention = `@${name} `;
  if (!textarea) {
    if (!form.content.includes(mention)) {
      form.content = `${form.content}${form.content ? "\n" : ""}${mention}`;
    }
    closeMentionPopup();
    return;
  }

  const cursor = textarea.selectionStart ?? form.content.length;
  const start = mentionStart.value ?? cursor;
  const end = textarea.selectionEnd ?? form.content.length;
  const nextContent = `${form.content.slice(0, start)}${mention}${form.content.slice(end)}`;
  form.content = nextContent;
  closeMentionPopup();

  void nextTick(() => {
    textarea.focus();
    const cursor = start + mention.length;
    textarea.setSelectionRange(cursor, cursor);
  });
}

function syncMentionState() {
  const textarea = textareaRef.value;
  if (!textarea) {
    closeMentionPopup();
    return;
  }

  const cursor = textarea.selectionStart ?? form.content.length;
  const beforeCursor = form.content.slice(0, cursor);
  const match = beforeCursor.match(/(^|\s)@([^\s@]*)$/);
  if (!match) {
    closeMentionPopup();
    return;
  }

  mentionQuery.value = match[2] ?? "";
  mentionStart.value = cursor - ((match[2] ?? "").length + 1);
  if (mentionIndex.value >= filteredMentionNames.value.length) {
    mentionIndex.value = 0;
  }
}

function closeMentionPopup() {
  mentionStart.value = null;
  mentionQuery.value = "";
  mentionIndex.value = 0;
}

function handleTextareaKeydown(event: KeyboardEvent) {
  if (!mentionPopupVisible.value) {
    return;
  }

  if (event.key === "ArrowDown") {
    event.preventDefault();
    mentionIndex.value = (mentionIndex.value + 1) % filteredMentionNames.value.length;
    return;
  }

  if (event.key === "ArrowUp") {
    event.preventDefault();
    mentionIndex.value = (mentionIndex.value - 1 + filteredMentionNames.value.length) % filteredMentionNames.value.length;
    return;
  }

  if (event.key === "Enter" || event.key === "Tab") {
    event.preventDefault();
    const current = filteredMentionNames.value[mentionIndex.value];
    if (current) {
      insertMention(current);
    }
    return;
  }

  if (event.key === "Escape") {
    closeMentionPopup();
  }
}

function startReply(id: number) {
  parentId.value = id;
  const findTarget = (list: CommentNode[]): CommentNode | undefined => {
    for (const item of list) {
      if (item.id === id) return item;
      if (item.replies?.length) {
        const nested = findTarget(item.replies);
        if (nested) return nested;
      }
    }
    return undefined;
  };
  const target = findTarget(comments.value);
  if (target?.author_name) {
    insertMention(target.author_name);
  }
  requestAnimationFrame(() => {
    document
      .getElementById("comment-form-area")
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  });
}

function beginEditEmail() {
  emailEditing.value = true;
}

function clearCachedEmail() {
  form.email = "";
  emailEditing.value = true;
  localStorage.removeItem(VISITOR_EMAIL_KEY);
}

function loadCachedEmail() {
  const cachedEmail = localStorage.getItem(VISITOR_EMAIL_KEY) || "";
  form.email = cachedEmail;
  emailEditing.value = cachedEmail === "";
}

function maskEmail(input: string) {
  const value = input.trim();
  if (!value.includes("@")) return value;
  const [localPart, domain = ""] = value.split("@");
  const localVisible = localPart.slice(0, Math.min(2, localPart.length));
  const maskedLocal = `${localVisible}${localPart.length > 2 ? "***" : "*"}`;
  const domainParts = domain.split(".");
  const mainDomain = domainParts[0] || "";
  const suffix = domainParts.length > 1 ? `.${domainParts.slice(1).join(".")}` : "";
  const domainVisible = mainDomain.slice(0, Math.min(1, mainDomain.length));
  return `${maskedLocal}@${domainVisible}${mainDomain.length > 1 ? "***" : "*"}${suffix}`;
}

onMounted(() => {
  loadCachedEmail();
  void loadComments();
});
</script>

<template>
  <div class="comment-widget">
    <div id="comment-form-area" class="comment-form">
      <div class="comment-form-top">
        <div class="comment-identity">
          <Avatar class="comment-identity-avatar" :size="42">
            {{ identityInitial }}
          </Avatar>
          <div class="comment-identity-body">
            <span>匿名身份</span>
            <strong>{{ identityName }}</strong>
          </div>
        </div>

        <Input
          v-if="showEmailInput"
          v-model="form.email"
          class="comment-email-input"
          placeholder="邮箱(可选,用于接收回复提醒)"
          allow-clear
          size="large"
        />
        <div v-else class="comment-email-memory">
          <div class="comment-email-memory-copy">
            <span>已记住联系邮箱</span>
            <strong>{{ maskedEmail }}</strong>
          </div>
          <div class="comment-email-memory-actions">
            <Button type="text" size="mini" @click="beginEditEmail">修改</Button>
            <Button type="text" size="mini" status="danger" @click="clearCachedEmail">清除</Button>
          </div>
        </div>

        <div class="comment-hint-box">
          {{ replyHint || "默认昵称已经和当前浏览器指纹绑定，可直接发表评论" }}
        </div>
      </div>

      <div class="comment-editor-shell">
        <textarea
          ref="textareaRef"
          v-model="form.content"
          class="comment-native-textarea"
          :placeholder="parentId ? '在此输入回复内容' : '支持多行评论,提交后进入待审核状态'"
          @input="syncMentionState"
          @click="syncMentionState"
          @keyup="syncMentionState"
          @keydown="handleTextareaKeydown"
        />
        <div v-if="mentionPopupVisible" class="comment-mention-popup">
          <button
            v-for="(name, index) in filteredMentionNames"
            :key="name"
            type="button"
            class="comment-mention-popup-item"
            :class="{ 'is-active': index === mentionIndex }"
            @mousedown.prevent="insertMention(name)"
          >
            @{{ name }}
          </button>
        </div>
        <Button
          class="comment-submit-float"
          type="primary"
          size="small"
          :loading="loading"
          @click="submit"
        >
          <template #icon><IconSend /></template>
          {{ parentId ? "提交回复" : "提交评论" }}
        </Button>
      </div>
      <div v-if="parentId" class="comment-form-foot">
        <span class="comment-form-hint">{{ replyHint }}</span>
        <Button
          size="small"
          @click="parentId = null"
        >
          取消回复
        </Button>
      </div>
    </div>

    <div class="comment-list">
      <CommentItem
        v-for="comment in comments"
        :key="comment.id"
        :comment="comment"
        @reply="startReply"
      />
      <Empty
        v-if="comments.length === 0"
        description="还没有通过审核的评论,成为第一个留言的人"
      />
    </div>
  </div>
</template>

<style scoped>
.comment-widget {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.comment-form-top {
  display: grid;
  grid-template-columns: minmax(180px, 0.8fr) minmax(220px, 1fr) minmax(220px, 1fr);
  gap: 10px;
  align-items: stretch;
}

.comment-identity {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--surface-subtle, #fafbfc);
  border: 1px solid var(--line, rgba(17, 17, 17, 0.08));
  border-radius: 12px;
}

.comment-identity-avatar {
  flex-shrink: 0;
  background: var(--primary-soft, rgba(22, 93, 255, 0.08)) !important;
  color: var(--primary, #165dff) !important;
  border: 1px solid rgba(22, 93, 255, 0.16);
  font-weight: 700;
}

.comment-identity-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.comment-identity-body span {
  color: var(--text-soft, #4e5969);
  font-size: 11px;
  letter-spacing: 0.02em;
}

.comment-identity-body strong {
  color: var(--text, #1d2129);
  font-size: 15px;
  line-height: 1.3;
  font-weight: 600;
}

.comment-email-input {
  align-self: stretch;
}

.comment-email-input :deep(.arco-input-wrapper) {
  height: 100%;
  min-height: 56px;
  border-radius: 12px;
  display: flex;
  align-items: center;
}

.comment-email-memory {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 56px;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--surface-subtle, #fafbfc);
  border: 1px solid var(--line, rgba(17, 17, 17, 0.08));
}

.comment-email-memory-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.comment-email-memory-copy span {
  color: var(--text-soft, #4e5969);
  font-size: 11px;
  letter-spacing: 0.02em;
}

.comment-email-memory-copy strong {
  color: var(--text, #1d2129);
  font-size: 14px;
  line-height: 1.4;
  word-break: break-all;
}

.comment-email-memory-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.comment-hint-box {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--surface-subtle, #fafbfc);
  border: 1px solid var(--line, rgba(17, 17, 17, 0.08));
  color: var(--text-soft, #4e5969);
  font-size: 12px;
  line-height: 1.55;
}

.comment-form {
  padding: 12px;
  background: var(--surface, rgba(255, 255, 255, 0.84));
  border: 1px solid var(--line, rgba(17, 17, 17, 0.08));
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.comment-editor-shell {
  position: relative;
}

.comment-native-textarea {
  width: 100%;
  min-height: 108px;
  resize: vertical;
  border: 1px solid rgba(17, 17, 17, 0.12);
  border-radius: 16px;
  padding: 14px 16px 54px;
  color: var(--text, #2f2a24);
  background: #fff;
  font: inherit;
  line-height: 1.7;
}

.comment-mention-popup {
  position: absolute;
  left: 12px;
  bottom: 12px;
  display: grid;
  gap: 6px;
  min-width: 220px;
  max-width: min(360px, calc(100% - 140px));
  padding: 8px;
  border: 1px solid var(--line, rgba(17, 17, 17, 0.08));
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.08);
}

.comment-mention-popup-item {
  display: flex;
  align-items: center;
  width: 100%;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--text, #1d2129);
  padding: 8px 10px;
  font: inherit;
  font-size: 13px;
  text-align: left;
}

.comment-mention-popup-item:hover,
.comment-mention-popup-item.is-active {
  background: rgba(22, 93, 255, 0.08);
  color: var(--primary, #165dff);
}

.comment-native-textarea:focus {
  outline: 0;
  border-color: #165dff;
  box-shadow: 0 0 0 2px rgba(22, 93, 255, 0.15);
}

.comment-submit-float {
  position: absolute;
  right: 12px;
  bottom: 12px;
  border-radius: 12px;
}

.comment-form-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: -2px;
}

.comment-form-hint {
  font-size: 12px;
  color: var(--muted, #5f5a54);
}

.comment-list {
  display: flex;
  flex-direction: column;
}

@media (max-width: 560px) {
  .comment-form-top {
    grid-template-columns: 1fr;
  }

  .comment-identity {
    align-items: flex-start;
  }

  .comment-email-memory {
    flex-direction: column;
    align-items: flex-start;
  }

  .comment-form :deep(.arco-input-wrapper) {
    width: 100% !important;
  }

  .comment-form-foot {
    flex-direction: column;
    align-items: flex-start;
  }

  .comment-mention-popup {
    left: 10px;
    right: 10px;
    max-width: none;
  }
}
</style>
