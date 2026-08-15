// Copy buttons for the API reference. Each button names the element holding
// its command; the shell prompt lives in its own span so what reaches the
// clipboard is directly runnable, the same rule the CLI section follows.

for (const btn of document.querySelectorAll('.api-copy')) {
  btn.addEventListener('click', async () => {
    const src = document.getElementById(btn.dataset.copy);
    if (!src) return;
    const prompt = src.querySelector('.install-prompt');
    const text = src.textContent.slice(prompt ? prompt.textContent.length : 0);
    try {
      await navigator.clipboard.writeText(text);
      btn.textContent = 'Copied ✓';
    } catch {
      btn.textContent = 'Press ⌘C';
    }
    setTimeout(() => { btn.textContent = 'Copy'; }, 1800);
  });
}
