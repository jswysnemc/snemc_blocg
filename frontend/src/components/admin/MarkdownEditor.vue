<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import Vditor from "vditor";

const props = defineProps<{
  modelValue: string;
  token: string;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const editorTarget = ref<HTMLDivElement | null>(null);
let editor: Vditor | null = null;

onMounted(() => {
  editor = new Vditor(editorTarget.value as HTMLDivElement, {
    mode: "ir",
    height: 620,
    cache: { enable: false },
    placeholder: "在这里撰写文章正文，支持 Markdown、公式、Mermaid 和图片上传。",
    counter: {
      enable: true,
      max: 50000,
    },
    toolbarConfig: {
      pin: true,
    },
    preview: {
      math: { engine: "KaTeX" },
      markdown: { toc: true },
      hljs: { enable: true, lineNumber: false },
      actions: [],
    },
    upload: {
      url: "/api/admin/media/images",
      linkToImgUrl: "/api/admin/media/import",
      fieldName: "image",
      multiple: true,
      accept: "image/jpeg,image/png,image/webp,image/gif",
      setHeaders() {
        return {
          Authorization: `Bearer ${props.token}`,
        };
      },
      validate(files) {
        for (const file of files) {
          if (file.size > 10 * 1024 * 1024) {
            return `${file.name} 超过 10MB 限制`;
          }
        }
        return true;
      },
    },
    value: props.modelValue,
    input(value) {
      emit("update:modelValue", value);
    },
  });
});

watch(
  () => props.modelValue,
  (value) => {
    if (!editor || editor.getValue() === value) {
      return;
    }
    editor.setValue(value);
  },
);

onBeforeUnmount(() => {
  editor?.destroy();
});
</script>

<template>
  <div ref="editorTarget" class="editor-surface"></div>
</template>
