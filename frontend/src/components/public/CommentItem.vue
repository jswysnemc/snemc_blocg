<script setup lang="ts">
import { computed, ref } from "vue";
import { Avatar, Button } from "@arco-design/web-vue";
import { IconHeart, IconHeartFill, IconMessage } from "@arco-design/web-vue/es/icon";
import type { CommentNode } from "../../types";

const props = defineProps<{
  comment: CommentNode;
}>();

const emit = defineEmits<{
  (event: "reply", parentId: number): void;
}>();

const likes = ref(props.comment.likes);
const liked = ref(props.comment.liked_by_visitor);

const initial = computed(() => {
  const name = props.comment.author_name?.trim() || "匿";
  return name.slice(0, 1).toUpperCase();
});

const timeText = computed(() => {
  if (!props.comment.created_at) return "";
  const d = new Date(props.comment.created_at);
  if (Number.isNaN(d.getTime())) return props.comment.created_at;
  return d.toLocaleString("zh-CN", { hour12: false });
});

async function likeComment() {
  if (liked.value) {
    return;
  }
  const response = await fetch(`/api/comments/${props.comment.id}/like`, {
    method: "POST",
  });
  const data = (await response.json()) as { likes: number; liked: boolean };
  likes.value = data.likes;
  liked.value = data.liked;
}
</script>

<template>
  <article class="comment-item">
    <div class="comment-item-avatar">
      <Avatar :size="32" :style="{ backgroundColor: '#165DFF' }">
        {{ initial }}
      </Avatar>
    </div>
    <div class="comment-item-body">
      <div class="comment-item-head">
        <strong>{{ comment.author_name || "匿名身份" }}</strong>
        <span class="comment-item-time">{{ timeText }}</span>
      </div>
      <p class="comment-item-text">{{ comment.content }}</p>
      <div class="comment-item-actions">
        <Button
          size="mini"
          type="text"
          :disabled="liked"
          @click="likeComment"
        >
          <template #icon>
            <IconHeartFill v-if="liked" style="color: rgb(var(--red-6))" />
            <IconHeart v-else />
          </template>
          {{ likes }}
        </Button>
        <Button size="mini" type="text" @click="emit('reply', comment.id)">
          <template #icon><IconMessage /></template>
          回复
        </Button>
      </div>
      <div v-if="comment.replies?.length" class="comment-item-children">
        <CommentItem
          v-for="reply in comment.replies"
          :key="reply.id"
          :comment="reply"
          @reply="emit('reply', $event)"
        />
      </div>
    </div>
  </article>
</template>

<style scoped>
.comment-item {
  display: flex;
  gap: 12px;
  padding: 12px 0;
  border-top: 1px solid var(--line, rgba(17, 17, 17, 0.08));
}

.comment-item:first-child {
  border-top: 0;
}

.comment-item-avatar {
  flex-shrink: 0;
}

.comment-item-body {
  flex: 1;
  min-width: 0;
}

.comment-item-head {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}

.comment-item-head strong {
  font-weight: 600;
  color: var(--text, #111);
}

.comment-item-time {
  color: var(--muted, #5f5a54);
  font-size: 12px;
}

.comment-item-text {
  margin: 6px 0 8px;
  color: var(--text, #2f2a24);
  font-size: 14px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.comment-item-actions {
  display: flex;
  gap: 4px;
  align-items: center;
}

.comment-item-children {
  margin-top: 12px;
  padding-left: 12px;
  border-left: 2px solid var(--line, rgba(17, 17, 17, 0.08));
}

.comment-item-children .comment-item {
  padding: 10px 0;
}
</style>
