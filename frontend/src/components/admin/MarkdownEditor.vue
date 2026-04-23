<script setup lang="ts">
import { Message } from "@arco-design/web-vue";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { createTyporaEditor } from "./typora-core";

type ToolbarState = {
  activeCommands: Record<string, boolean>;
  tableToolsVisible: boolean;
};

type EditorStats = {
  blocks: number;
  characters: number;
  label: string;
};

const props = defineProps<{
  modelValue: string;
  token: string;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
  (event: "editor-scroll", scrollTop: number): void;
}>();

const editorHost = ref<HTMLDivElement | null>(null);
const imageInput = ref<HTMLInputElement | null>(null);
const statusText = ref("Markdown 实时同步");
const statsText = ref("0 块 · 0 字符");
const tableToolsVisible = ref(false);
const activeCommands = ref<Record<string, boolean>>({});
let editor: ReturnType<typeof createTyporaEditor> | null = null;
let lastMarkdown = props.modelValue;
let pasteHandler: ((event: ClipboardEvent) => void) | null = null;
let dragOverHandler: ((event: DragEvent) => void) | null = null;
let dropHandler: ((event: DragEvent) => void) | null = null;
let scrollHandler: (() => void) | null = null;

const commandGroups = [
  [
    { key: "bold", label: "加粗" },
    { key: "italic", label: "斜体" },
    { key: "strike", label: "删除线" },
    { key: "inlineCode", label: "行内代码" },
  ],
  [
    { key: "h1", label: "H1" },
    { key: "h2", label: "H2" },
    { key: "quote", label: "引用" },
    { key: "bulletList", label: "无序列表" },
    { key: "orderedList", label: "有序列表" },
    { key: "table", label: "表格" },
    { key: "codeBlock", label: "代码块" },
    { key: "mathBlock", label: "公式" },
    { key: "mermaidBlock", label: "Mermaid" },
  ],
];

const tableCommands = [
  { key: "addRowBefore", label: "前插行" },
  { key: "addRowAfter", label: "后插行" },
  { key: "addColumnBefore", label: "前插列" },
  { key: "addColumnAfter", label: "后插列" },
  { key: "toggleHeaderRow", label: "表头行" },
  { key: "deleteRow", label: "删行" },
  { key: "deleteColumn", label: "删列" },
  { key: "deleteTable", label: "删表" },
];

function preventToolbarBlur(event: PointerEvent) {
  const target = event.target as HTMLElement | null;
  if (target?.closest("button")) {
    event.preventDefault();
  }
}

function handleToolbarState(nextState: ToolbarState) {
  activeCommands.value = nextState.activeCommands;
  tableToolsVisible.value = nextState.tableToolsVisible;
}

function handleStats(nextStats: EditorStats) {
  statsText.value = nextStats.label;
}

function handleChange(markdown: string) {
  lastMarkdown = markdown;
  emit("update:modelValue", markdown);
}

function runCommand(command: string) {
  editor?.runCommand(command);
}

function runTableCommand(command: string) {
  editor?.runTableCommand(command);
}

async function copyMarkdown() {
  try {
    await editor?.copyMarkdown();
  } catch {
    Message.error("复制失败");
  }
}

function downloadMarkdown() {
  editor?.downloadMarkdown();
}

function openImagePicker() {
  imageInput.value?.click();
}

async function uploadImage(file: File) {
  const formData = new FormData();
  formData.append("image", file);
  const response = await fetch("/api/admin/media/images", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${props.token}`,
    },
    body: formData,
  });
  const payload = await response.json();
  if (!response.ok || payload?.code !== 0) {
    throw new Error(payload?.msg || "上传失败");
  }

  const asset = payload?.assets?.[0];
  const url = asset?.markdown_url || Object.values(payload?.data?.succMap || {})[0];
  if (typeof url !== "string" || !url) {
    throw new Error("上传结果缺少图片地址");
  }
  return url;
}

async function handleImageSelection(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) {
    return;
  }
  await insertImageFile(file);
}

async function insertImageFile(file: File) {
  try {
    const url = await uploadImage(file);
    editor?.insertImage(url, { alt: file.name });
  } catch (error) {
    Message.error(error instanceof Error ? error.message : "上传失败");
  }
}

async function insertRemoteImage() {
  const rawURL = window.prompt("输入远程图片地址");
  if (!rawURL?.trim()) {
    return;
  }

  try {
    const response = await fetch("/api/admin/media/import", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${props.token}`,
      },
      body: JSON.stringify({ url: rawURL.trim() }),
    });
    const payload = await response.json();
    if (!response.ok || payload?.code !== 0) {
      throw new Error(payload?.msg || "远程图片接入失败");
    }
    const url = payload?.data?.url;
    if (typeof url !== "string" || !url) {
      throw new Error("远程图片地址无效");
    }
    editor?.insertImage(url);
  } catch (error) {
    Message.error(error instanceof Error ? error.message : "远程图片接入失败");
  }
}

onMounted(() => {
  if (!editorHost.value) {
    return;
  }
  editor = createTyporaEditor({
    element: editorHost.value,
    markdown: props.modelValue,
    onChange: handleChange,
    onStatusChange: (message: string) => {
      statusText.value = message;
    },
    onStatsChange: handleStats,
    onToolbarChange: handleToolbarState,
  });

  pasteHandler = (event: ClipboardEvent) => {
    const item = Array.from(event.clipboardData?.items || []).find((entry) =>
      entry.type.startsWith("image/"),
    );
    const file = item?.getAsFile();
    if (!file) {
      return;
    }
    event.preventDefault();
    void insertImageFile(file);
  };

  dragOverHandler = (event: DragEvent) => {
    if ((event.dataTransfer?.files?.length || 0) > 0) {
      event.preventDefault();
    }
  };

  dropHandler = (event: DragEvent) => {
    const file = Array.from(event.dataTransfer?.files || []).find((entry) =>
      entry.type.startsWith("image/"),
    );
    if (!file) {
      return;
    }
    event.preventDefault();
    editor?.focus();
    void insertImageFile(file);
  };

  scrollHandler = () => {
    emit("editor-scroll", editorHost.value?.scrollTop ?? 0);
  };

  editorHost.value.addEventListener("paste", pasteHandler);
  editorHost.value.addEventListener("dragover", dragOverHandler);
  editorHost.value.addEventListener("drop", dropHandler);
  editorHost.value.addEventListener("scroll", scrollHandler, { passive: true });
  scrollHandler();
});

watch(
  () => props.modelValue,
  (value) => {
    if (!editor || value === lastMarkdown) {
      return;
    }
    lastMarkdown = value;
    editor.setMarkdown(value);
  },
);

onBeforeUnmount(() => {
  if (editorHost.value && pasteHandler && dragOverHandler && dropHandler) {
    editorHost.value.removeEventListener("paste", pasteHandler);
    editorHost.value.removeEventListener("dragover", dragOverHandler);
    editorHost.value.removeEventListener("drop", dropHandler);
  }
  if (editorHost.value && scrollHandler) {
    editorHost.value.removeEventListener("scroll", scrollHandler);
  }
  editor?.destroy();
});
</script>

<template>
  <section class="typora-editor">
    <div class="typora-toolbar" @pointerdown="preventToolbarBlur">
      <template v-for="(group, groupIndex) in commandGroups" :key="groupIndex">
        <button
          v-for="command in group"
          :key="command.key"
          type="button"
          class="typora-tool"
          :data-active="activeCommands[command.key] ? 'true' : 'false'"
          @click="runCommand(command.key)"
        >
          {{ command.label }}
        </button>
        <span
          v-if="groupIndex < commandGroups.length - 1"
          :key="`divider-${groupIndex}`"
          class="typora-toolbar-divider"
        ></span>
      </template>

      <span class="typora-toolbar-divider"></span>

      <button type="button" class="typora-tool secondary" @click="openImagePicker">
        图片
      </button>
      <button type="button" class="typora-tool secondary" @click="insertRemoteImage">
        远程图
      </button>
      <button type="button" class="typora-tool secondary" @click="copyMarkdown">
        复制 Markdown
      </button>
      <button type="button" class="typora-tool secondary" @click="downloadMarkdown">
        导出 .md
      </button>
    </div>

    <div class="typora-editor-card">
      <div class="typora-editor-meta" @pointerdown="preventToolbarBlur">
        <div class="typora-meta-readout">
          <span>{{ statusText }}</span>
          <span>{{ statsText }}</span>
        </div>

        <div v-if="tableToolsVisible" class="table-toolbar">
          <button
            v-for="command in tableCommands"
            :key="command.key"
            type="button"
            @click="runTableCommand(command.key)"
          >
            {{ command.label }}
          </button>
        </div>
      </div>

      <div ref="editorHost" class="editor-host prose"></div>
    </div>

    <input
      ref="imageInput"
      type="file"
      accept="image/jpeg,image/png,image/webp,image/gif"
      hidden
      @change="handleImageSelection"
    />
  </section>
</template>
