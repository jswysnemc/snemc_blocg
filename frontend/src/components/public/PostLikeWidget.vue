<script setup lang="ts">
import { ref } from "vue";
import { Button } from "@arco-design/web-vue";
import { IconHeart, IconHeartFill } from "@arco-design/web-vue/es/icon";

const props = defineProps<{
  slug: string;
  initialLikes: number;
  initiallyLiked: boolean;
}>();

const likes = ref(props.initialLikes);
const liked = ref(props.initiallyLiked);
const loading = ref(false);

async function like() {
  if (loading.value || liked.value) {
    return;
  }
  loading.value = true;
  try {
    const response = await fetch(`/api/posts/${props.slug}/like`, {
      method: "POST",
    });
    const data = (await response.json()) as { likes: number; liked: boolean };
    likes.value = data.likes;
    liked.value = data.liked;
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <Button
    :type="liked ? 'outline' : 'primary'"
    size="medium"
    :loading="loading"
    :disabled="liked"
    @click="like"
  >
    <template #icon>
      <IconHeartFill v-if="liked" style="color: rgb(var(--red-6))" />
      <IconHeart v-else />
    </template>
    {{ liked ? "已点赞" : "点赞文章" }} · {{ likes }}
  </Button>
</template>
