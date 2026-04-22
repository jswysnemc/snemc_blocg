// @ts-nocheck

import "./typora-editor.css";

import markdownIt from "markdown-it";
import Prism from "prismjs";
import "prismjs/components/prism-bash";
import "prismjs/components/prism-css";
import "prismjs/components/prism-java";
import "prismjs/components/prism-javascript";
import "prismjs/components/prism-json";
import "prismjs/components/prism-jsx";
import "prismjs/components/prism-markdown";
import "prismjs/components/prism-markup";
import "prismjs/components/prism-python";
import "prismjs/components/prism-sql";
import "prismjs/components/prism-tsx";
import "prismjs/components/prism-typescript";
import "prismjs/components/prism-yaml";
import "prismjs/components/prism-go";
import { baseKeymap, chainCommands, createParagraphNear, exitCode, liftEmptyBlock, setBlockType, splitBlock, toggleMark, wrapIn } from "prosemirror-commands";
import { undo, redo, history } from "prosemirror-history";
import { inputRules, textblockTypeInputRule, wrappingInputRule } from "prosemirror-inputrules";
import { keymap } from "prosemirror-keymap";
import { Schema } from "prosemirror-model";
import { MarkdownParser, MarkdownSerializer, defaultMarkdownParser, defaultMarkdownSerializer, schema as baseMarkdownSchema } from "prosemirror-markdown";
import { EditorState, Selection, TextSelection } from "prosemirror-state";
import { EditorView } from "prosemirror-view";
import { liftListItem, sinkListItem, splitListItem, wrapInList } from "prosemirror-schema-list";
import {
  addColumnAfter,
  addColumnBefore,
  addRowAfter,
  addRowBefore,
  columnResizing,
  deleteColumn,
  deleteRow,
  deleteTable,
  fixTables,
  goToNextCell,
  isInTable,
  tableEditing,
  tableNodes,
  TableView,
  toggleHeaderRow,
} from "prosemirror-tables";

export const SAMPLE_DOCUMENT = `# Web Typora

这个 demo 不是“左边源码、右边预览”，而是单视图的 Markdown 编辑体验。

## 你可以直接输入

- 在行首输入 Markdown 快捷语法
- 用工具栏切换加粗、斜体和行内代码
- 在代码块里直接编辑源码，同时实时高亮

> 重点不在于把 Markdown 文本塞进一个大 textarea，而是把文档建成真正的结构化编辑器。

## 代码块只有一个编辑面

在空行输入 \`\`\`ts 然后按回车，会创建一个代码块。之后你看到的是一个代码区域，输入和高亮都发生在这个区域内部，不会再出现上下两个“源码 / 预览”窗口。

\`\`\`ts
export function renderEditor(mode: "wysiwyg" | "raw") {
  if (mode === "wysiwyg") {
    return "single-surface editing";
  }

  return "split preview";
}
\`\`\`

## 为什么这个方案更像 Typora

1. 标题、列表、引用都是结构化节点，不需要每次全文重新渲染。
2. 代码块使用单节点视图，输入层和高亮层叠加在同一个区域。
3. 文档始终可以导出为 Markdown。

## 表格也是结构化节点

| 模式 | 是否分栏 | 编辑感受 |
| :--- | :---: | ---: |
| 普通 Markdown 预览器 | 是 | 容易来回跳 |
| 这个 demo | 否 | 更接近 Typora |
| 代码块编辑 | 单区域 | 实时高亮 |`;

const editorSchema = new Schema({
  nodes: baseMarkdownSchema.spec.nodes.append(
    tableNodes({
      tableGroup: "block",
      cellContent: "paragraph+",
      cellAttributes: {
        align: {
          default: null,
          getFromDOM(dom) {
            return dom.getAttribute("data-align") || dom.style.textAlign || null;
          },
          setDOMAttr(value, attrs) {
            if (!value) {
              return;
            }

            attrs["data-align"] = value;
            attrs.style = attrs.style ? `${attrs.style};text-align:${value}` : `text-align:${value}`;
          },
        },
      },
    }),
  ),
  marks: baseMarkdownSchema.spec.marks,
});

const markdownTokenizer = markdownIt("commonmark", { html: false, linkify: true }).enable("table");
const markdownParser = new MarkdownParser(editorSchema, markdownTokenizer, {
  ...defaultMarkdownParser.tokens,
  table: { block: "table" },
  thead: { ignore: true },
  tbody: { ignore: true },
  tr: { block: "table_row" },
  th: { block: "table_header" },
  td: { block: "table_cell" },
});

markdownParser.tokenHandlers.th_open = (state, token) => {
  state.openNode(editorSchema.nodes.table_header, getTableCellAttrs(token));
  state.openNode(editorSchema.nodes.paragraph);
};

markdownParser.tokenHandlers.th_close = (state) => {
  state.closeNode();
  state.closeNode();
};

markdownParser.tokenHandlers.td_open = (state, token) => {
  state.openNode(editorSchema.nodes.table_cell, getTableCellAttrs(token));
  state.openNode(editorSchema.nodes.paragraph);
};

markdownParser.tokenHandlers.td_close = (state) => {
  state.closeNode();
  state.closeNode();
};

const markdownSerializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    table(state, node) {
      renderMarkdownTable(state, node);
    },
  },
  defaultMarkdownSerializer.marks,
);

const COMMANDS = {
  bold: toggleMark(editorSchema.marks.strong),
  italic: toggleMark(editorSchema.marks.em),
  inlineCode: toggleMark(editorSchema.marks.code),
  h1: toggleTextBlock(editorSchema.nodes.heading, { level: 1 }),
  h2: toggleTextBlock(editorSchema.nodes.heading, { level: 2 }),
  quote: wrapIn(editorSchema.nodes.blockquote),
  bulletList: wrapInList(editorSchema.nodes.bullet_list),
  orderedList: wrapInList(editorSchema.nodes.ordered_list),
  table: insertTable(),
  codeBlock: toggleCodeBlock(),
};

const TABLE_COMMANDS = {
  addRowBefore,
  addRowAfter,
  addColumnBefore,
  addColumnAfter,
  deleteRow,
  deleteColumn,
  toggleHeaderRow,
  deleteTable,
};

const LANGUAGE_ALIASES = {
  html: "markup",
  xml: "markup",
  svg: "markup",
  vue: "markup",
  js: "javascript",
  ts: "typescript",
  sh: "bash",
  shell: "bash",
  yml: "yaml",
  md: "markdown",
  plaintext: "plain",
  text: "plain",
};

const TABLE_MIN_ROWS = 1;
const TABLE_MIN_COLUMNS = 1;
const TABLE_DEFAULT_CELL_MIN_WIDTH = 124;
const TABLE_PICKER_ROWS = 10;
const TABLE_PICKER_COLUMNS = 8;

let view;
let statusTimer = 0;
let persistentStatus = "Markdown 实时同步";
let statusCallback = null;
let statsCallback = null;
let toolbarCallback = null;
let changeCallback = null;
let readyCallback = null;

class CodeBlockView {
  constructor(node, editorView, getPos) {
    this.node = node;
    this.view = editorView;
    this.getPos = getPos;
    this.updating = false;

    this.dom = document.createElement("section");
    this.dom.className = "pm-code-block";

    this.header = document.createElement("div");
    this.header.className = "code-block-header";

    this.fenceLabel = document.createElement("span");
    this.fenceLabel.className = "code-fence-label";
    this.fenceLabel.textContent = "```";

    this.paramsInput = document.createElement("input");
    this.paramsInput.className = "code-language-input";
    this.paramsInput.type = "text";
    this.paramsInput.autocomplete = "off";
    this.paramsInput.spellcheck = false;
    this.paramsInput.placeholder = "language";
    this.paramsInput.value = node.attrs.params || "";

    this.headerHint = document.createElement("span");
    this.headerHint.className = "code-header-hint";
    this.headerHint.textContent = "Tab 缩进 / Ctrl+Enter 退出";

    this.header.append(this.fenceLabel, this.paramsInput, this.headerHint);

    this.surface = document.createElement("div");
    this.surface.className = "code-surface";

    this.highlight = document.createElement("pre");
    this.highlight.className = "code-highlight";
    this.highlight.setAttribute("aria-hidden", "true");

    this.code = document.createElement("code");
    this.highlight.append(this.code);

    this.textarea = document.createElement("textarea");
    this.textarea.className = "code-editor";
    this.textarea.value = node.textContent;
    this.textarea.spellcheck = false;
    this.textarea.setAttribute("aria-label", "代码块编辑区域");

    this.surface.append(this.highlight, this.textarea);
    this.dom.append(this.header, this.surface);

    this.paramsInput.addEventListener("input", () => this.updateParams());
    this.paramsInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        this.textarea.focus();
      }
    });

    this.textarea.addEventListener("input", () => {
      this.renderHighlight();
      this.adjustHeight();
      this.syncScroll();
      this.forwardText();
    });

    this.textarea.addEventListener("scroll", () => this.syncScroll());
    this.textarea.addEventListener("focus", () => this.forwardSelection());
    this.textarea.addEventListener("blur", () => this.renderHighlight());
    this.textarea.addEventListener("click", () => this.forwardSelection());
    this.textarea.addEventListener("keyup", () => this.forwardSelection());
    this.textarea.addEventListener("select", () => this.forwardSelection());
    this.textarea.addEventListener("keydown", (event) => this.handleKeydown(event));

    this.renderHighlight();
    this.adjustHeight();
  }

  update(node) {
    if (node.type !== this.node.type) {
      return false;
    }

    this.node = node;

    if (this.paramsInput.value !== (node.attrs.params || "")) {
      this.paramsInput.value = node.attrs.params || "";
    }

    if (!this.updating && this.textarea.value !== node.textContent) {
      this.textarea.value = node.textContent;
    }

    this.renderHighlight();
    this.adjustHeight();
    return true;
  }

  selectNode() {
    this.textarea.focus();
  }

  setSelection(anchor, head) {
    this.updating = true;
    this.textarea.focus();
    this.textarea.setSelectionRange(anchor, head);
    this.updating = false;
    this.renderHighlight();
  }

  stopEvent() {
    return true;
  }

  ignoreMutation() {
    return true;
  }

  updateParams() {
    const params = this.paramsInput.value;

    if (params === (this.node.attrs.params || "")) {
      return;
    }

    const position = this.getPos();
    const transaction = this.view.state.tr.setNodeMarkup(position, null, {
      ...this.node.attrs,
      params,
    });

    this.view.dispatch(transaction);
  }

  forwardText() {
    if (this.updating) {
      return;
    }

    const previous = this.node.textContent;
    const next = this.textarea.value;
    const offset = this.getPos() + 1;
    const { selectionStart, selectionEnd } = this.textarea;

    let start = 0;
    let previousEnd = previous.length;
    let nextEnd = next.length;

    while (start < previousEnd && start < nextEnd && previous.charCodeAt(start) === next.charCodeAt(start)) {
      start += 1;
    }

    while (
      previousEnd > start &&
      nextEnd > start &&
      previous.charCodeAt(previousEnd - 1) === next.charCodeAt(nextEnd - 1)
    ) {
      previousEnd -= 1;
      nextEnd -= 1;
    }

    let transaction = this.view.state.tr;

    if (previousEnd > start || nextEnd > start) {
      if (nextEnd > start) {
        transaction = transaction.replaceWith(
          offset + start,
          offset + previousEnd,
          this.view.state.schema.text(next.slice(start, nextEnd)),
        );
      } else {
        transaction = transaction.delete(offset + start, offset + previousEnd);
      }
    }

    transaction = transaction.setSelection(TextSelection.create(transaction.doc, offset + selectionStart, offset + selectionEnd));
    this.view.dispatch(transaction);
  }

  forwardSelection() {
    this.renderHighlight();

    if (this.updating) {
      return;
    }

    const offset = this.getPos() + 1;
    const from = offset + this.textarea.selectionStart;
    const to = offset + this.textarea.selectionEnd;
    const selection = this.view.state.selection;

    if (selection.from === from && selection.to === to) {
      return;
    }

    this.view.dispatch(this.view.state.tr.setSelection(TextSelection.create(this.view.state.doc, from, to)));
  }

  handleKeydown(event) {
    if (event.key === "Tab") {
      event.preventDefault();
      const { selectionStart, selectionEnd } = this.textarea;
      this.textarea.setRangeText("  ", selectionStart, selectionEnd, "end");
      this.renderHighlight();
      this.adjustHeight();
      this.forwardText();
      return;
    }

    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      this.forwardSelection();

      if (!exitCode(this.view.state, this.view.dispatch)) {
        const position = this.getPos() + this.node.nodeSize;
        const paragraph = this.view.state.schema.nodes.paragraph.create();
        let transaction = this.view.state.tr.insert(position, paragraph);
        transaction = transaction.setSelection(TextSelection.create(transaction.doc, position + 1)).scrollIntoView();
        this.view.dispatch(transaction);
      }

      this.view.focus();
      return;
    }

    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "z" && !event.shiftKey) {
      event.preventDefault();
      undo(this.view.state, this.view.dispatch);
      return;
    }

    if (
      ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "z" && event.shiftKey) ||
      ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "y")
    ) {
      event.preventDefault();
      redo(this.view.state, this.view.dispatch);
      return;
    }

    if (event.key === "ArrowUp" && this.maybeEscape("line", -1)) {
      event.preventDefault();
      return;
    }

    if (event.key === "ArrowDown" && this.maybeEscape("line", 1)) {
      event.preventDefault();
      return;
    }

    if (event.key === "ArrowLeft" && this.maybeEscape("char", -1)) {
      event.preventDefault();
      return;
    }

    if (event.key === "ArrowRight" && this.maybeEscape("char", 1)) {
      event.preventDefault();
    }
  }

  maybeEscape(unit, direction) {
    const start = this.textarea.selectionStart;
    const end = this.textarea.selectionEnd;

    if (start !== end) {
      return false;
    }

    if (unit === "char") {
      if (direction < 0 ? start > 0 : end < this.textarea.value.length) {
        return false;
      }
    } else {
      const lineStart = this.textarea.value.lastIndexOf("\n", start - 1) + 1;
      const nextBreak = this.textarea.value.indexOf("\n", start);
      const lineEnd = nextBreak === -1 ? this.textarea.value.length : nextBreak;

      if (direction < 0 ? lineStart > 0 : lineEnd < this.textarea.value.length) {
        return false;
      }
    }

    const targetPosition = this.getPos() + (direction < 0 ? 0 : this.node.nodeSize);
    const selection = Selection.near(this.view.state.doc.resolve(targetPosition), direction);
    const transaction = this.view.state.tr.setSelection(selection).scrollIntoView();
    this.view.dispatch(transaction);
    this.view.focus();
    return true;
  }

  adjustHeight() {
    this.textarea.style.height = "0px";
    const height = Math.max(this.textarea.scrollHeight, 152);
    this.surface.style.height = `${height}px`;
    this.textarea.style.height = `${height}px`;
  }

  syncScroll() {
    this.highlight.scrollTop = this.textarea.scrollTop;
    this.highlight.scrollLeft = this.textarea.scrollLeft;
  }

  renderHighlight() {
    const params = this.paramsInput.value.trim();
    const language = resolveLanguage(params);
    const content = this.textarea.value;
    const hasFocus = document.activeElement === this.textarea;
    const selectionStart = hasFocus ? this.textarea.selectionStart : 0;
    const selectionEnd = hasFocus ? this.textarea.selectionEnd : 0;

    this.dom.dataset.language = params || "plain text";
    this.surface.dataset.empty = content.length === 0 ? "true" : "false";

    if (!language || !Prism.languages[language]) {
      this.code.innerHTML = renderSelectedPlainText(content, selectionStart, selectionEnd) || "<span class=\"token plain\"> </span>";
      return;
    }

    this.code.innerHTML =
      renderHighlightedCode(content, Prism.languages[language], selectionStart, selectionEnd) ||
      "<span class=\"token plain\"> </span>";
  }
}

class AdjustableTableView {
  constructor(node, editorView, getPos) {
    this.node = node;
    this.view = editorView;
    this.getPos = getPos;
    this.innerView = new TableView(node, TABLE_DEFAULT_CELL_MIN_WIDTH);
    this.isOpen = false;
    this.isSelecting = false;
    this.previewRows = getTableRowCount(node);
    this.previewColumns = getTableColumnCount(node);

    this.dom = document.createElement("div");
    this.dom.className = "pm-table-shell";

    this.handle = document.createElement("button");
    this.handle.type = "button";
    this.handle.className = "table-corner-trigger";
    this.handle.setAttribute("aria-label", "调整表格行列");
    this.handle.setAttribute("aria-expanded", "false");
    this.handle.innerHTML = `
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <rect x="1.5" y="2.5" width="13" height="11" rx="1.6"></rect>
        <path d="M1.5 6.5h13M1.5 10.5h13M5.8 2.5v11M10.2 2.5v11"></path>
      </svg>
    `;

    this.panel = document.createElement("div");
    this.panel.className = "table-corner-panel";
    this.panel.hidden = true;

    this.grid = document.createElement("div");
    this.grid.className = "table-corner-grid";

    this.footer = document.createElement("div");
    this.footer.className = "table-corner-footer";

    this.dimension = document.createElement("span");
    this.dimension.className = "table-corner-dimension";

    this.footer.append(this.dimension);
    this.panel.append(this.grid, this.footer);
    this.dom.append(this.handle, this.panel, this.innerView.dom);
    this.contentDOM = this.innerView.contentDOM;

    this.handle.addEventListener("click", () => {
      this.setOpen(!this.isOpen);
    });

    this.panel.addEventListener("mouseleave", () => {
      if (this.isOpen && !this.isSelecting) {
        this.resetPreview();
      }
    });

    this.panel.addEventListener("pointerdown", (event) => {
      const cell = event.target.closest(".table-corner-cell");
      if (!cell) {
        return;
      }

      event.preventDefault();
      this.isSelecting = true;
      this.previewRows = Number.parseInt(cell.dataset.rows, 10);
      this.previewColumns = Number.parseInt(cell.dataset.columns, 10);
      this.paintPicker();
    });

    this.panel.addEventListener("pointerover", (event) => {
      if (!this.isSelecting) {
        return;
      }

      const cell = event.target.closest(".table-corner-cell");
      if (!cell) {
        return;
      }

      this.previewRows = Number.parseInt(cell.dataset.rows, 10);
      this.previewColumns = Number.parseInt(cell.dataset.columns, 10);
      this.paintPicker();
    });

    this.panel.addEventListener("pointerup", (event) => {
      if (!this.isSelecting) {
        return;
      }

      event.preventDefault();
      this.isSelecting = false;
      this.applyResize(this.previewRows, this.previewColumns);
    });

    this.handleDocumentPointerDown = (event) => {
      if (!this.isOpen || this.dom.contains(event.target)) {
        return;
      }

      this.setOpen(false);
    };

    this.handleDocumentPointerUp = (event) => {
      if (!this.isOpen || !this.isSelecting) {
        return;
      }

      if (this.dom.contains(event.target)) {
        return;
      }

      this.isSelecting = false;
      this.applyResize(this.previewRows, this.previewColumns);
    };

    this.view.dom.ownerDocument.addEventListener("pointerdown", this.handleDocumentPointerDown);
    this.view.dom.ownerDocument.addEventListener("pointerup", this.handleDocumentPointerUp);

    this.syncPicker();
  }

  update(node) {
    if (!this.innerView.update(node)) {
      return false;
    }

    this.node = node;
    this.syncPicker();
    return true;
  }

  ignoreMutation(record) {
    if (this.handle.contains(record.target) || this.panel.contains(record.target)) {
      return true;
    }

    return this.innerView.ignoreMutation(record);
  }

  stopEvent(event) {
    return this.handle.contains(event.target) || this.panel.contains(event.target);
  }

  destroy() {
    this.view.dom.ownerDocument.removeEventListener("pointerdown", this.handleDocumentPointerDown);
    this.view.dom.ownerDocument.removeEventListener("pointerup", this.handleDocumentPointerUp);
  }

  setOpen(open) {
    this.isOpen = open;
    this.isSelecting = false;
    this.panel.hidden = !open;
    this.handle.setAttribute("aria-expanded", String(open));

    if (open) {
      this.syncPicker();
    } else {
      this.resetPreview();
    }
  }

  syncPicker() {
    const rows = getTableRowCount(this.node);
    const columns = getTableColumnCount(this.node);

    this.previewRows = rows;
    this.previewColumns = columns;
    this.renderPickerGrid();
    this.paintPicker();
  }

  resetPreview() {
    this.previewRows = getTableRowCount(this.node);
    this.previewColumns = getTableColumnCount(this.node);
    this.paintPicker();
  }

  renderPickerGrid() {
    const rows = Math.max(TABLE_PICKER_ROWS, getTableRowCount(this.node));
    const columns = Math.max(TABLE_PICKER_COLUMNS, getTableColumnCount(this.node));

    if (this.grid.dataset.rows === String(rows) && this.grid.dataset.columns === String(columns)) {
      return;
    }

    this.grid.dataset.rows = String(rows);
    this.grid.dataset.columns = String(columns);
    this.grid.style.setProperty("--picker-columns", String(columns));
    this.grid.textContent = "";

    for (let rowIndex = 1; rowIndex <= rows; rowIndex += 1) {
      for (let columnIndex = 1; columnIndex <= columns; columnIndex += 1) {
        const cell = document.createElement("button");
        cell.type = "button";
        cell.className = "table-corner-cell";
        cell.dataset.rows = String(rowIndex);
        cell.dataset.columns = String(columnIndex);
        cell.setAttribute("aria-label", `${columnIndex} 列 ${rowIndex} 行`);

        cell.addEventListener("mouseenter", () => {
          this.previewRows = rowIndex;
          this.previewColumns = columnIndex;
          this.paintPicker();
        });

        cell.addEventListener("focus", () => {
          this.previewRows = rowIndex;
          this.previewColumns = columnIndex;
          this.paintPicker();
        });

        cell.addEventListener("keydown", (event) => {
          if (event.key !== "Enter" && event.key !== " ") {
            return;
          }

          event.preventDefault();
          this.previewRows = rowIndex;
          this.previewColumns = columnIndex;
          this.paintPicker();
          this.applyResize(rowIndex, columnIndex);
        });

        this.grid.append(cell);
      }
    }
  }

  paintPicker() {
    this.grid.querySelectorAll(".table-corner-cell").forEach((cell) => {
      const active =
        Number.parseInt(cell.dataset.rows, 10) <= this.previewRows &&
        Number.parseInt(cell.dataset.columns, 10) <= this.previewColumns;

      cell.dataset.active = active ? "true" : "false";
    });

    this.dimension.textContent = `${this.previewColumns} × ${this.previewRows}`;
  }

  applyResize(rows, columns) {
    const nextRows = clampTableSize(rows, TABLE_MIN_ROWS);
    const nextColumns = clampTableSize(columns, TABLE_MIN_COLUMNS);

    if (nextRows === getTableRowCount(this.node) && nextColumns === getTableColumnCount(this.node)) {
      this.setOpen(false);
      return;
    }

    const position = this.getPos();
    const nextTable = resizeTableNode(this.node, nextRows, nextColumns);
    let transaction = this.view.state.tr.replaceWith(position, position + this.node.nodeSize, nextTable);
    transaction = transaction
      .setSelection(TextSelection.create(transaction.doc, getTableCellTextPosition(position, nextTable, Math.min(1, nextRows - 1), 0)))
      .scrollIntoView();
    this.view.dispatch(transaction);
    this.setOpen(false);
    this.view.focus();
  }
}

function getTableCellAttrs(token) {
  const style = token.attrGet?.("style") || "";
  const match = /text-align:\s*(left|center|right)/i.exec(style);

  return {
    align: match ? match[1].toLowerCase() : null,
  };
}

function createTableCell(nodeType, text = "", attrs = null) {
  const paragraph = editorSchema.nodes.paragraph.create(
    null,
    text ? editorSchema.text(text) : null,
  );

  return nodeType.createAndFill(attrs, paragraph);
}

function createTableNode(rows = 3, columns = 3, options = {}) {
  const { headerLabels = null, alignments = [] } = options;
  const rowNodes = [];

  for (let rowIndex = 0; rowIndex < rows; rowIndex += 1) {
    const cellType = rowIndex === 0 ? editorSchema.nodes.table_header : editorSchema.nodes.table_cell;
    const cells = [];

    for (let columnIndex = 0; columnIndex < columns; columnIndex += 1) {
      const label =
        rowIndex === 0
          ? (headerLabels?.[columnIndex] ?? `列 ${columnIndex + 1}`)
          : "";
      const align = alignments[columnIndex] || null;
      cells.push(createTableCell(cellType, label, { align }));
    }

    rowNodes.push(editorSchema.nodes.table_row.create(null, cells));
  }

  return editorSchema.nodes.table.create(null, rowNodes);
}

function getTableRowCount(tableNode) {
  return tableNode.childCount;
}

function getTableColumnCount(tableNode) {
  const firstRow = tableNode.firstChild;

  if (!firstRow) {
    return 0;
  }

  let count = 0;
  firstRow.forEach((cell) => {
    count += cell.attrs.colspan || 1;
  });
  return count;
}

function clampTableSize(value, minimum) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? Math.max(parsed, minimum) : minimum;
}

function getTableCellTextPosition(tablePos, tableNode, rowIndex = 0, columnIndex = 0) {
  const safeRowIndex = Math.min(Math.max(rowIndex, 0), Math.max(tableNode.childCount - 1, 0));
  const rowNode = tableNode.child(safeRowIndex);
  const safeColumnIndex = Math.min(Math.max(columnIndex, 0), Math.max(rowNode.childCount - 1, 0));

  let position = tablePos + 1;

  for (let index = 0; index < safeRowIndex; index += 1) {
    position += tableNode.child(index).nodeSize;
  }

  position += 1;

  for (let index = 0; index < safeColumnIndex; index += 1) {
    position += rowNode.child(index).nodeSize;
  }

  return position + 1;
}

function resizeTableNode(tableNode, nextRows, nextColumns) {
  const alignments = Array.from({ length: nextColumns }, (_, index) => getColumnAlignment(tableNode, index));
  const rows = [];

  for (let rowIndex = 0; rowIndex < nextRows; rowIndex += 1) {
    const sourceRow = rowIndex < tableNode.childCount ? tableNode.child(rowIndex) : null;
    const cellType = rowIndex === 0 ? editorSchema.nodes.table_header : editorSchema.nodes.table_cell;
    const cells = [];

    for (let columnIndex = 0; columnIndex < nextColumns; columnIndex += 1) {
      const sourceCell = sourceRow && columnIndex < sourceRow.childCount ? sourceRow.child(columnIndex) : null;

      if (sourceCell) {
        cells.push(
          cellType.createAndFill(
            {
              ...sourceCell.attrs,
              align: sourceCell.attrs.align ?? alignments[columnIndex] ?? null,
              colspan: 1,
              rowspan: 1,
              colwidth: null,
            },
            sourceCell.content,
          ),
        );
      } else {
        const label = rowIndex === 0 ? `列 ${columnIndex + 1}` : "";
        cells.push(createTableCell(cellType, label, { align: alignments[columnIndex] || null }));
      }
    }

    rows.push(editorSchema.nodes.table_row.create(null, cells));
  }

  return editorSchema.nodes.table.create(tableNode.attrs, rows);
}

function insertTable(rows = 3, columns = 3) {
  return (state, dispatch) => {
    if (!dispatch) {
      return true;
    }

    const { $from } = state.selection;
    const topLevelNode = $from.node(1);
    const replaceEmptyParagraph =
      topLevelNode.type === editorSchema.nodes.paragraph &&
      topLevelNode.childCount === 0;
    const insertPos = replaceEmptyParagraph ? $from.before(1) : $from.after(1);
    const table = createTableNode(rows, columns);

    let transaction = state.tr;

    if (replaceEmptyParagraph) {
      transaction = transaction.replaceWith(insertPos, insertPos + topLevelNode.nodeSize, table);
    } else {
      transaction = transaction.insert(insertPos, table);
    }

    transaction = transaction
      .setSelection(TextSelection.create(transaction.doc, getTableCellTextPosition(insertPos, table, 1, 0)))
      .scrollIntoView();
    dispatch(transaction);
    return true;
  };
}

function serializeTableCell(node) {
  const fragmentDoc = editorSchema.nodes.doc.create(null, node.content);
  const markdown = defaultMarkdownSerializer.serialize(fragmentDoc).trim();
  const flattened = markdown.replace(/\n{2,}/g, "<br>").replace(/\n/g, "<br>");

  return flattened.replace(/\|/g, "\\|");
}

function getColumnAlignment(tableNode, columnIndex) {
  for (let rowIndex = 0; rowIndex < tableNode.childCount; rowIndex += 1) {
    const row = tableNode.child(rowIndex);
    const cell = row.child(columnIndex);

    if (cell?.attrs.align) {
      return cell.attrs.align;
    }
  }

  return null;
}

function createDividerCell(align, width) {
  const hyphenCount = Math.max(width, 3);

  if (align === "left") {
    return `:${"-".repeat(hyphenCount)}`;
  }

  if (align === "right") {
    return `${"-".repeat(hyphenCount)}:`;
  }

  if (align === "center") {
    return `:${"-".repeat(hyphenCount)}:`;
  }

  return "-".repeat(hyphenCount);
}

function splitMarkdownTableCells(text) {
  const line = text.trim();

  if (!line.includes("|")) {
    return null;
  }

  let normalized = line;

  if (normalized.startsWith("|")) {
    normalized = normalized.slice(1);
  }

  if (normalized.endsWith("|")) {
    normalized = normalized.slice(0, -1);
  }

  const cells = normalized.split("|").map((cell) => cell.trim());
  return cells.length > 0 ? cells : null;
}

function parseMarkdownTableAlignment(text) {
  const cells = splitMarkdownTableCells(text);

  if (!cells || cells.length === 0) {
    return null;
  }

  const alignments = [];

  for (const cell of cells) {
    if (!/^:?-{3,}:?$/.test(cell)) {
      return null;
    }

    if (cell.startsWith(":") && cell.endsWith(":")) {
      alignments.push("center");
    } else if (cell.startsWith(":")) {
      alignments.push("left");
    } else if (cell.endsWith(":")) {
      alignments.push("right");
    } else {
      alignments.push(null);
    }
  }

  return alignments;
}

function markdownTableHandler() {
  return (state, dispatch) => {
    if (!state.selection.empty) {
      return false;
    }

    const { $from } = state.selection;

    if ($from.parent.type !== editorSchema.nodes.paragraph || $from.depth !== 1) {
      return false;
    }

    const currentIndex = $from.index(0);

    if (currentIndex === 0) {
      return false;
    }

    const currentNode = state.doc.child(currentIndex);
    const previousNode = state.doc.child(currentIndex - 1);

    if (previousNode.type !== editorSchema.nodes.paragraph) {
      return false;
    }

    const headerCells = splitMarkdownTableCells(previousNode.textContent);
    const alignments = parseMarkdownTableAlignment(currentNode.textContent);

    if (!headerCells || !alignments || headerCells.length !== alignments.length) {
      return false;
    }

    const currentPos = $from.before(1);
    const previousPos = currentPos - previousNode.nodeSize;
    const table = createTableNode(2, headerCells.length, { headerLabels: headerCells, alignments });

    if (dispatch) {
      let transaction = state.tr.replaceWith(previousPos, currentPos + currentNode.nodeSize, table);
      transaction = transaction
        .setSelection(TextSelection.create(transaction.doc, getTableCellTextPosition(previousPos, table, 1, 0)))
        .scrollIntoView();
      dispatch(transaction);
    }

    return true;
  };
}

function renderTableLine(cells, widths) {
  return `| ${cells.map((cell, index) => cell.padEnd(widths[index], " ")).join(" | ")} |`;
}

function renderMarkdownTable(state, node) {
  const rows = [];

  node.forEach((row) => {
    const cells = [];
    row.forEach((cell) => {
      cells.push(serializeTableCell(cell));
    });
    rows.push(cells);
  });

  const columnCount = rows[0]?.length || 1;
  const normalizedRows = rows.map((row) =>
    Array.from({ length: columnCount }, (_, index) => row[index] || ""),
  );
  const alignments = Array.from({ length: columnCount }, (_, index) => getColumnAlignment(node, index));
  const widths = Array.from({ length: columnCount }, (_, index) =>
    Math.max(
      3,
      ...normalizedRows.map((row) => row[index].length),
      createDividerCell(alignments[index], 3).length,
    ),
  );

  state.write(renderTableLine(normalizedRows[0], widths));
  state.write("\n");
  state.write(
    renderTableLine(
      alignments.map((align, index) => createDividerCell(align, widths[index])),
      widths,
    ),
  );

  for (let rowIndex = 1; rowIndex < normalizedRows.length; rowIndex += 1) {
    state.write("\n");
    state.write(renderTableLine(normalizedRows[rowIndex], widths));
  }

  state.closeBlock(node);
}

function toggleTextBlock(nodeType, attrs) {
  return (state, dispatch) => {
    const { $from } = state.selection;
    const sameType = $from.parent.type === nodeType;
    const sameAttrs = Object.entries(attrs || {}).every(([key, value]) => $from.parent.attrs[key] === value);

    if (sameType && sameAttrs) {
      return setBlockType(editorSchema.nodes.paragraph)(state, dispatch);
    }

    return setBlockType(nodeType, attrs)(state, dispatch);
  };
}

function toggleCodeBlock() {
  return (state, dispatch) => {
    if (state.selection.$from.parent.type === editorSchema.nodes.code_block) {
      return setBlockType(editorSchema.nodes.paragraph)(state, dispatch);
    }

    return setBlockType(editorSchema.nodes.code_block, { params: "ts" })(state, dispatch);
  };
}

function buildInputRules() {
  const rules = [
    textblockTypeInputRule(/^(#{1,6})\s$/, editorSchema.nodes.heading, (match) => ({ level: match[1].length })),
    wrappingInputRule(/^\s*>\s$/, editorSchema.nodes.blockquote),
    wrappingInputRule(/^\s*([-+*])\s$/, editorSchema.nodes.bullet_list),
    wrappingInputRule(
      /^(\d+)\.\s$/,
      editorSchema.nodes.ordered_list,
      (match) => ({ order: Number.parseInt(match[1], 10) || 1 }),
      (match, node) => node.childCount + node.attrs.order === Number.parseInt(match[1], 10),
    ),
  ];

  return inputRules({ rules });
}

function arrowHandler(direction) {
  return (state, dispatch, editorView) => {
    if (isInTable(state) || !state.selection.empty || !editorView.endOfTextblock(direction)) {
      return false;
    }

    const side = direction === "left" || direction === "up" ? -1 : 1;
    const head = state.selection.$head;
    const nextSelection = Selection.near(
      state.doc.resolve(side > 0 ? head.after() : head.before()),
      side,
    );

    if (nextSelection.$head && nextSelection.$head.parent.type.name === "code_block") {
      if (dispatch) {
        dispatch(state.tr.setSelection(nextSelection));
      }

      return true;
    }

    return false;
  };
}

function codeFenceHandler() {
  return (state, dispatch) => {
    const { $from } = state.selection;
    const parent = $from.parent;

    if (!state.selection.empty || parent.type !== editorSchema.nodes.paragraph) {
      return false;
    }

    const match = /^```([^\s`][^`]*)?$/.exec(parent.textContent.trim());

    if (!match) {
      return false;
    }

    const position = $from.before();
    const codeBlock = editorSchema.nodes.code_block.create({ params: match[1] || "" });
    let transaction = state.tr.replaceWith(position, position + parent.nodeSize, codeBlock);
    transaction = transaction.setSelection(TextSelection.create(transaction.doc, position + 1)).scrollIntoView();

    if (dispatch) {
      dispatch(transaction);
    }

    return true;
  };
}

function resolveLanguage(params) {
  if (!params) {
    return "";
  }

  const first = params.split(/\s+/)[0].toLowerCase();
  return LANGUAGE_ALIASES[first] || first;
}

function escapeHtml(text) {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#39;");
}

function wrapCodeSelection(text) {
  if (!text) {
    return "";
  }

  return `<span class="code-selection">${escapeHtml(text)}</span>`;
}

function renderSelectedPlainText(text, selectionStart, selectionEnd) {
  if (!text) {
    return "";
  }

  if (selectionStart === selectionEnd) {
    return escapeHtml(text);
  }

  const boundaries = [0, text.length];

  if (selectionStart > 0 && selectionStart < text.length) {
    boundaries.push(selectionStart);
  }

  if (selectionEnd > 0 && selectionEnd < text.length) {
    boundaries.push(selectionEnd);
  }

  boundaries.sort((left, right) => left - right);

  let html = "";

  for (let index = 0; index < boundaries.length - 1; index += 1) {
    const start = boundaries[index];
    const end = boundaries[index + 1];
    const chunk = text.slice(start, end);

    if (!chunk) {
      continue;
    }

    html += start >= selectionStart && end <= selectionEnd
      ? wrapCodeSelection(chunk)
      : escapeHtml(chunk);
  }

  return html;
}

function renderPrismContent(content, state) {
  if (typeof content === "string") {
    const html = renderSelectedPlainText(content, state.selectionStart - state.offset, state.selectionEnd - state.offset);
    state.offset += content.length;
    return html;
  }

  if (Array.isArray(content)) {
    return content.map((item) => renderPrismContent(item, state)).join("");
  }

  const classNames = ["token", content.type];
  const aliases = Array.isArray(content.alias) ? content.alias : content.alias ? [content.alias] : [];
  classNames.push(...aliases);

  return `<span class="${classNames.join(" ")}">${renderPrismContent(content.content, state)}</span>`;
}

function renderHighlightedCode(text, grammar, selectionStart, selectionEnd) {
  const tokens = Prism.tokenize(text, grammar);
  return renderPrismContent(tokens, {
    offset: 0,
    selectionStart,
    selectionEnd,
  });
}

function serializeMarkdown() {
  return markdownSerializer.serialize(view.state.doc);
}

function createState(markdown) {
  let state = EditorState.create({
    doc: markdownParser.parse(markdown),
    plugins: [
      history(),
      buildInputRules(),
      keymap({
        Enter: chainCommands(markdownTableHandler(), codeFenceHandler(), splitListItem(editorSchema.nodes.list_item), createParagraphNear, liftEmptyBlock, splitBlock),
        Tab: chainCommands(goToNextCell(1), sinkListItem(editorSchema.nodes.list_item)),
        "Shift-Tab": chainCommands(goToNextCell(-1), liftListItem(editorSchema.nodes.list_item)),
        "Mod-z": undo,
        "Shift-Mod-z": redo,
        "Mod-y": redo,
      }),
      keymap({
        ArrowLeft: arrowHandler("left"),
        ArrowRight: arrowHandler("right"),
        ArrowUp: arrowHandler("up"),
        ArrowDown: arrowHandler("down"),
      }),
      columnResizing(),
      tableEditing(),
      keymap(baseKeymap),
    ],
  });

  const fix = fixTables(state);
  return fix ? state.apply(fix.setMeta("addToHistory", false)) : state;
}

function setPersistentStatus(message) {
  persistentStatus = message;
  if (statusCallback) {
    statusCallback(message, false);
  }
}

function flashStatus(message) {
  window.clearTimeout(statusTimer);
  if (statusCallback) {
    statusCallback(message, true);
  }
  statusTimer = window.setTimeout(() => {
    if (statusCallback) {
      statusCallback(persistentStatus, false);
    }
  }, 1400);
}

function updateStats(state) {
  const characters = state.doc.textContent.replace(/\s+/g, "").length;
  if (statsCallback) {
    statsCallback({
      blocks: state.doc.childCount,
      characters,
      label: `${state.doc.childCount} 块 · ${characters} 字符`,
    });
  }
  updateToolbarState(state);
}

function hasAncestor($pos, nodeType) {
  for (let depth = $pos.depth; depth > 0; depth -= 1) {
    if ($pos.node(depth).type === nodeType) {
      return true;
    }
  }

  return false;
}

function isMarkActive(state, markType) {
  const { from, $from, to, empty } = state.selection;

  if (empty) {
    return Boolean(markType.isInSet(state.storedMarks || $from.marks()));
  }

  return state.doc.rangeHasMark(from, to, markType);
}

function updateToolbarState(state) {
  const { $from } = state.selection;
  const inTable = isInTable(state);
  const buttonMap = {
    bold: isMarkActive(state, editorSchema.marks.strong),
    italic: isMarkActive(state, editorSchema.marks.em),
    inlineCode: isMarkActive(state, editorSchema.marks.code),
    h1: $from.parent.type === editorSchema.nodes.heading && $from.parent.attrs.level === 1,
    h2: $from.parent.type === editorSchema.nodes.heading && $from.parent.attrs.level === 2,
    quote: hasAncestor($from, editorSchema.nodes.blockquote),
    bulletList: hasAncestor($from, editorSchema.nodes.bullet_list),
    orderedList: hasAncestor($from, editorSchema.nodes.ordered_list),
    table: false,
    codeBlock: $from.parent.type === editorSchema.nodes.code_block,
  };

  if (toolbarCallback) {
    toolbarCallback({
      activeCommands: buttonMap,
      tableToolsVisible: inTable,
    });
  }
}

function runCommand(command) {
  const handler = COMMANDS[command];

  if (!handler) {
    return;
  }

  handler(view.state, view.dispatch, view);
  view.focus();
}

function runTableCommand(command) {
  const handler = TABLE_COMMANDS[command];

  if (!handler) {
    return;
  }

  handler(view.state, view.dispatch, view);
  view.focus();
}

function replaceDocument(markdown) {
  view.updateState(createState(markdown));
  updateStats(view.state);
  if (changeCallback) {
    changeCallback(serializeMarkdown(), { source: "replace" });
  }
  view.focus();
}

async function copyMarkdown() {
  const markdown = serializeMarkdown();
  await navigator.clipboard.writeText(markdown);
  flashStatus("Markdown 已复制");
}

function downloadMarkdown() {
  const blob = new Blob([serializeMarkdown()], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = "web-typora-demo.md";
  anchor.click();
  URL.revokeObjectURL(url);
  flashStatus("Markdown 已导出");
}

function insertImage(url, options = {}) {
  const trimmed = (url || "").trim();
  if (!trimmed || !view) {
    return false;
  }

  const imageNode = editorSchema.nodes.image.create({
    src: trimmed,
    alt: options.alt || "",
    title: options.title || null,
  });
  const transaction = view.state.tr.replaceSelectionWith(imageNode).scrollIntoView();
  view.dispatch(transaction);
  view.focus();
  flashStatus("图片已插入");
  return true;
}

export function createTyporaEditor(options) {
  const {
    element,
    markdown = "",
    onChange,
    onStatusChange,
    onStatsChange,
    onToolbarChange,
    onReady,
  } = options;

  if (!element) {
    throw new Error("editor host is required");
  }

  if (view) {
    view.destroy();
  }

  statusCallback = onStatusChange || null;
  statsCallback = onStatsChange || null;
  toolbarCallback = onToolbarChange || null;
  changeCallback = onChange || null;
  readyCallback = onReady || null;
  persistentStatus = "Markdown 实时同步";
  element.innerHTML = "";

  view = new EditorView(element, {
    state: createState(markdown),
    nodeViews: {
      table(node, editorView, getPos) {
        return new AdjustableTableView(node, editorView, getPos);
      },
      code_block(node, editorView, getPos) {
        return new CodeBlockView(node, editorView, getPos);
      },
    },
    dispatchTransaction(transaction) {
      const nextState = view.state.apply(transaction);
      view.updateState(nextState);
      updateStats(nextState);

      if (transaction.docChanged && changeCallback) {
        changeCallback(serializeMarkdown(), { source: "input" });
      }
    },
  });

  setPersistentStatus(persistentStatus);
  updateStats(view.state);
  if (readyCallback) {
    readyCallback({
      markdown: serializeMarkdown(),
    });
  }

  return {
    focus() {
      view.focus();
    },
    destroy() {
      window.clearTimeout(statusTimer);
      view?.destroy();
      view = null;
      statusCallback = null;
      statsCallback = null;
      toolbarCallback = null;
      changeCallback = null;
      readyCallback = null;
    },
    getMarkdown() {
      return serializeMarkdown();
    },
    setMarkdown(nextMarkdown) {
      if (!view) {
        return;
      }
      if (nextMarkdown === serializeMarkdown()) {
        return;
      }
      replaceDocument(nextMarkdown);
    },
    runCommand,
    runTableCommand,
    replaceDocument,
    copyMarkdown,
    downloadMarkdown,
    insertImage,
    flashStatus,
  };
}
