import type { PostSummary } from "./types";

const VISITOR_KEY = "snemc-blog-visitor-id";
const VISITOR_NAME_KEY = "snemc-blog-visitor-name";

void boot();

async function boot() {
  await initVisitor();
  initCopyLink();
  initTrackPageView();
  initArchiveFeed();
  initSearchForms();
  initSidebarToggle();
  await Promise.all([initArticleEnhancements(), mountWidgets()]);
}

async function mountWidgets() {
  const postLikeNodes = document.querySelectorAll<HTMLElement>('[data-widget="post-like"]');
  const commentNodes = document.querySelectorAll<HTMLElement>('[data-widget="comments"]');
  if (postLikeNodes.length === 0 && commentNodes.length === 0) {
    return;
  }

  const [{ createApp }, commentModule, likeModule] = await Promise.all([
    import("vue"),
    commentNodes.length ? import("./components/public/CommentWidget.vue") : Promise.resolve(null),
    postLikeNodes.length ? import("./components/public/PostLikeWidget.vue") : Promise.resolve(null),
  ]);
  await import("./styles/theme.css");
  await import("./public-enhance.css");

  if (likeModule) {
    postLikeNodes.forEach((node) => {
      const slug = node.dataset.slug ?? "";
      const initialLikes = Number(node.dataset.likes ?? "0");
      const initiallyLiked = node.dataset.liked === "true";
      createApp(likeModule.default, { slug, initialLikes, initiallyLiked }).mount(node);
    });
  }

  if (commentModule) {
    commentNodes.forEach((node) => {
      const slug = node.dataset.postSlug ?? "";
      createApp(commentModule.default, { slug }).mount(node);
    });
  }
}

async function initVisitor() {
  const visitorId = getOrCreateVisitorID();
  const fingerprint = await buildFingerprint();
  const response = await fetch("/api/visitor/init", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      visitor_id: visitorId,
      fingerprint,
      language: navigator.language,
    }),
  });
  const data = (await response.json()) as {
    visitor_id?: string;
    display_name?: string;
  };
  if (data.visitor_id) {
    localStorage.setItem(VISITOR_KEY, data.visitor_id);
  }
  if (data.display_name) {
    localStorage.setItem(VISITOR_NAME_KEY, data.display_name);
  }
}

function getOrCreateVisitorID() {
  let current = localStorage.getItem(VISITOR_KEY);
  if (!current) {
    current = crypto.randomUUID();
    localStorage.setItem(VISITOR_KEY, current);
  }
  return current;
}

async function buildFingerprint() {
  const raw = [
    navigator.userAgent,
    navigator.language,
    `${screen.width}x${screen.height}`,
    Intl.DateTimeFormat().resolvedOptions().timeZone,
    String(navigator.hardwareConcurrency ?? ""),
  ].join("|");
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw));
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

function initTrackPageView() {
  const path = window.location.pathname;
  const parts = path.split("/");
  const slug = parts[1] === "posts" ? parts[2] : "";
  void fetch("/api/track/pageview", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      path,
      slug,
      referrer: document.referrer,
    }),
  });
}

function initCopyLink() {
  document.querySelectorAll<HTMLButtonElement>("[data-copy-link]").forEach((button) => {
    button.addEventListener("click", async () => {
      const url = button.dataset.copyLink ?? window.location.href;
      await navigator.clipboard.writeText(url);
      const original = button.textContent;
      button.textContent = "已复制";
      setTimeout(() => {
        button.textContent = original;
      }, 1200);
    });
  });
}

async function initArticleEnhancements() {
  const root = document.getElementById("article-content");
  if (!root) {
    return;
  }

  const module = await import("./article-enhancements");
  await module.enhanceArticle(root);
}

function initArchiveFeed() {
  const root = document.querySelector<HTMLElement>("[data-archive-feed]");
  if (!root) {
    return;
  }

  const grid = root.querySelector<HTMLElement>("[data-archive-grid]");
  const progress = root.querySelector<HTMLElement>("[data-archive-progress]");
  const complete = root.querySelector<HTMLElement>("[data-archive-complete]");
  const button = root.querySelector<HTMLButtonElement>("[data-archive-load-more]");
  const sentinel = root.querySelector<HTMLElement>("[data-archive-sentinel]");
  if (!grid || !progress || !complete || !button || !sentinel) {
    return;
  }

  let pageSize = Number(root.dataset.pageSize ?? "8");
  let loaded = Number(root.dataset.loaded ?? "0");
  let total = Number(root.dataset.total ?? "0");
  let hasMore = loaded < total;
  let loading = false;
  const idleLabel = button.textContent || "加载更多";

  const updateUI = () => {
    progress.textContent = `已显示 ${loaded} / ${total} 篇文章`;
    button.hidden = !hasMore;
    complete.hidden = hasMore;
  };

  const loadMore = async () => {
    if (loading || !hasMore) {
      return;
    }
    loading = true;
    button.disabled = true;
    button.textContent = "加载中...";
    try {
      const response = await fetch(`/api/archive/posts?offset=${loaded}&limit=${pageSize}`);
      if (!response.ok) {
        throw new Error(`archive request failed: ${response.status}`);
      }
      const data = (await response.json()) as {
        posts: PostSummary[];
        total: number;
        next_offset: number;
        has_more: boolean;
      };
      appendArchivePosts(grid, data.posts);
      loaded = data.next_offset;
      total = data.total;
      hasMore = data.has_more;
      updateUI();
    } catch (error) {
      console.error(error);
      button.textContent = "加载失败，重试";
      button.disabled = false;
      loading = false;
      return;
    }
    button.textContent = idleLabel;
    button.disabled = false;
    loading = false;
  };

  button.addEventListener("click", () => {
    void loadMore();
  });

  if ("IntersectionObserver" in window) {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          void loadMore();
        }
      });
    }, { rootMargin: "320px 0px" });
    observer.observe(sentinel);
  }

  updateUI();
}

function initSearchForms() {
  document.querySelectorAll<HTMLFormElement>("[data-search-form]").forEach((form) => {
    const submit = form.querySelector<HTMLButtonElement>("[data-search-submit]");
    const status = form.parentElement?.querySelector<HTMLElement>("[data-search-status]");
    if (!submit || !status) {
      return;
    }

    const idleLabel = submit.textContent || "检索";
    form.addEventListener("submit", () => {
      const mode = (form.querySelector<HTMLInputElement>('input[name="mode"]:checked')?.value || "keyword").trim();
      const text = mode === "semantic" ? "语义搜索中..." : "关键词检索中...";
      submit.disabled = true;
      submit.classList.add("is-loading");
      submit.textContent = text;
      status.hidden = false;
      status.textContent = text;
      window.setTimeout(() => {
        submit.disabled = false;
        submit.classList.remove("is-loading");
        submit.textContent = idleLabel;
      }, 12000);
    });
  });
}

function initSidebarToggle() {
  const toggle = document.querySelector<HTMLButtonElement>(".sidebar-toggle");
  const content = document.getElementById("sidebar-content");
  if (!toggle || !content) {
    return;
  }

  toggle.addEventListener("click", () => {
    const isExpanded = toggle.getAttribute("aria-expanded") === "true";
    toggle.setAttribute("aria-expanded", String(!isExpanded));
    content.classList.toggle("is-open");
  });
}

function appendArchivePosts(container: HTMLElement, posts: PostSummary[]) {
  if (posts.length === 0) {
    return;
  }
  container.querySelector(".empty-text")?.remove();
  const html = posts.map(renderPostCard).join("");
  container.insertAdjacentHTML("beforeend", html);
}

function renderPostCard(post: PostSummary) {
  const summary = post.summary || post.excerpt || "";
  const tags = (post.tags || [])
    .map((tag) => `<a class="tag" href="/tag/${encodeURIComponent(tag.slug)}">#${escapeHTML(tag.name)}</a>`)
    .join("");
  const category = post.category_name
    ? `<a class="pill" href="/category/${encodeURIComponent(post.category_slug)}">${escapeHTML(post.category_name)}</a>`
    : "";
  return `<article class="post-card">
    <div class="post-card-top">
      ${category}
      <span class="stat">${Number(post.reading_time || 0)} min</span>
    </div>
    <h3><a href="/posts/${encodeURIComponent(post.slug)}">${escapeHTML(post.title)}</a></h3>
    <p>${escapeHTML(summary)}</p>
    <div class="post-card-bottom">
      <div class="tag-row">${tags}</div>
      <div class="meta-row">
        <span>${Number(post.views || 0)} views</span>
        <span>${Number(post.likes || 0)} likes</span>
      </div>
    </div>
  </article>`;
}

function escapeHTML(input: string) {
  return input
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll(`"`, "&quot;")
    .replaceAll("'", "&#39;");
}
