import mermaid from "mermaid";

export async function renderMermaid(root: HTMLElement) {
  mermaid.initialize({
    startOnLoad: false,
    theme: "neutral",
  });
  await mermaid.run({
    nodes: Array.from(root.querySelectorAll(".mermaid")),
  });
}
