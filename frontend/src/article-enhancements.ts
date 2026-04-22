import renderMathInElement from "katex/contrib/auto-render";
import "katex/dist/katex.min.css";
import "./public-enhance.css";

export async function enhanceArticle(root: HTMLElement) {
  renderMathInElement(root, {
    delimiters: [
      { left: "$$", right: "$$", display: true },
      { left: "$", right: "$", display: false },
      { left: "\\(", right: "\\)", display: false },
      { left: "\\[", right: "\\]", display: true },
    ],
    throwOnError: false,
  });

  initCodeBlocks(root);
  initTOC(root);

  if (root.querySelector(".mermaid")) {
    const module = await import("./mermaid-enhancement");
    await module.renderMermaid(root);
  }
}

function initCodeBlocks(root: HTMLElement) {
  root.querySelectorAll<HTMLElement>(".code-block").forEach((block) => {
    if (block.querySelector(".code-block-toolbar")) {
      return;
    }
    const languageMeta = resolveLanguageMeta(block.dataset.language ?? "");
    const pre = block.querySelector<HTMLElement>("pre");
    const code = block.querySelector<HTMLElement>("code");
    if (!pre || !code) {
      return;
    }

    const toolbar = document.createElement("div");
    toolbar.className = "code-block-toolbar";

    const languageLabel = document.createElement("span");
    languageLabel.className = "code-block-language";
    const icon = document.createElement("span");
    icon.className = "code-block-icon";
    icon.textContent = languageMeta.icon;

    const text = document.createElement("span");
    text.className = "code-block-language-text";
    text.textContent = languageMeta.label;

    languageLabel.append(icon, text);

    const copyButton = document.createElement("button");
    copyButton.type = "button";
    copyButton.className = "code-copy-button";
    copyButton.textContent = "复制";
    copyButton.addEventListener("click", async () => {
      await navigator.clipboard.writeText(code.innerText);
      copyButton.textContent = "已复制";
      window.setTimeout(() => {
        copyButton.textContent = "复制";
      }, 1400);
    });

    toolbar.append(languageLabel, copyButton);
    block.prepend(toolbar);
  });
}

function resolveLanguageMeta(language: string) {
  const map: Record<string, { label: string; icon: string }> = {
    go: { label: "GO", icon: "" },
    js: { label: "JavaScript", icon: "" },
    ts: { label: "TypeScript", icon: "" },
    py: { label: "Python", icon: "" },
    rb: { label: "Ruby", icon: "" },
    rs: { label: "Rust", icon: "" },
    sh: { label: "Shell", icon: "" },
    bash: { label: "Bash", icon: "" },
    zsh: { label: "Zsh", icon: "" },
    md: { label: "Markdown", icon: "" },
    yml: { label: "YAML", icon: "" },
    yaml: { label: "YAML", icon: "" },
    json: { label: "JSON", icon: "" },
    sql: { label: "SQL", icon: "" },
    html: { label: "HTML", icon: "" },
    css: { label: "CSS", icon: "" },
    vue: { label: "Vue", icon: "" },
    mermaid: { label: "Mermaid", icon: "󰈺" },
  };
  const key = language.trim().toLowerCase();
  if (!key) {
    return { label: "Text", icon: "󰆍" };
  }
  return map[key] ?? {
    label: key.length <= 3 ? key.toUpperCase() : key[0].toUpperCase() + key.slice(1),
    icon: "󰆍",
  };
}

function initTOC(root: HTMLElement) {
  const target = document.querySelector<HTMLElement>("[data-toc]");
  if (!target) {
    return;
  }
  const preview = document.querySelector<HTMLElement>("[data-toc-preview]");
  try {
    const toc = JSON.parse(target.dataset.toc ?? "[]") as Array<{ id: string; text: string; level: number }>;
    const items = toc
      .map((item) => ({
        ...item,
        preview: extractSectionPreview(root.querySelector<HTMLElement>(`#${CSS.escape(item.id)}`), item.text),
      }));

    target.innerHTML = items
      .map((item) => `<a href="#${item.id}" data-id="${item.id}" title="${escapeHTML(item.preview)}" style="padding-left:${(item.level - 2) * 14}px">${escapeHTML(item.text)}</a>`)
      .join("");

    const links = Array.from(target.querySelectorAll<HTMLAnchorElement>("a"));
    const headings = items
      .map((item) => document.getElementById(item.id))
      .filter((node): node is HTMLElement => Boolean(node));

    const setActive = (id: string) => {
      links.forEach((link) => {
        link.classList.toggle("is-active", link.dataset.id === id);
      });
      const current = items.find((item) => item.id === id);
      if (preview && current) {
        preview.innerHTML = `<strong>${escapeHTML(current.text)}</strong><p>${escapeHTML(current.preview)}</p>`;
      }
    };

    links.forEach((link) => {
      link.addEventListener("mouseenter", () => {
        if (link.dataset.id) {
          setActive(link.dataset.id);
        }
      });
      link.addEventListener("click", (event) => {
        event.preventDefault();
        const id = link.dataset.id;
        if (!id) {
          return;
        }
        document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
        setActive(id);
      });
    });

    if (items[0]) {
      setActive(items[0].id);
    }

    const observer = new IntersectionObserver((entries) => {
      const visible = entries
        .filter((entry) => entry.isIntersecting)
        .sort((a, b) => b.intersectionRatio - a.intersectionRatio);
      if (visible[0]?.target instanceof HTMLElement) {
        setActive(visible[0].target.id);
      }
    }, {
      rootMargin: "-18% 0px -58% 0px",
      threshold: [0.1, 0.3, 0.6, 1],
    });

    headings.forEach((heading) => observer.observe(heading));
  } catch {
    target.innerHTML = "";
  }
}

function extractSectionPreview(heading: HTMLElement | null, fallback: string) {
  if (!heading) {
    return fallback;
  }
  let cursor = heading.nextElementSibling;
  const chunks: string[] = [];
  while (cursor && !/^H[1-6]$/.test(cursor.tagName)) {
    const text = cursor.textContent?.trim();
    if (text) {
      chunks.push(text);
    }
    if (chunks.join(" ").length > 120) {
      break;
    }
    cursor = cursor.nextElementSibling;
  }
  const summary = chunks.join(" ").trim() || fallback;
  return summary.length > 120 ? `${summary.slice(0, 120)}...` : summary;
}

function escapeHTML(input: string) {
  return input
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
