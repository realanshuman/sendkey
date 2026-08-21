// The numbers page: read /api/stats and lay the counters out. Everything
// here is presentation; the server's totals are the single source of truth
// and the page never computes a number the API did not send.

const $ = (id) => document.getElementById(id);

const REFRESH_MS = 15_000;
const reduced = matchMedia('(prefers-reduced-motion: reduce)');

// human turns raw counts into the shape people actually read at a glance,
// keeping one decimal only where it earns its place (1.2k, 3.4 MB).
function human(n) {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`;
  return `${(n / 1_000_000).toFixed(1)}m`;
}
function humanBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

// countTo animates a number climbing in chunky steps() spirit: 16 discrete
// frames, not a glide. Under prefers-reduced-motion it just sets the text.
const shown = new Map(); // element id -> last value, so refreshes animate the delta
function countTo(el, target, fmt) {
  const from = shown.get(el.id) ?? 0;
  shown.set(el.id, target);
  if (reduced.matches || target === from) {
    el.textContent = fmt(target);
    return;
  }
  const steps = 16;
  let i = 0;
  const tick = () => {
    i++;
    const v = Math.round(from + ((target - from) * i) / steps);
    el.textContent = fmt(v);
    if (i < steps) setTimeout(tick, 34);
  };
  tick();
}

function setBar(barId, parts) {
  const total = parts.reduce((n, [, v]) => n + v, 0);
  const bar = $(barId);
  for (const [key, v] of parts) {
    const seg = bar.querySelector(`[data-seg="${key}"]`);
    const pct = total ? (v / total) * 100 : 0;
    seg.style.width = `${pct}%`;
    const label = document.querySelector(`[data-pct="${key}"]`);
    if (label) label.textContent = total ? `${Math.round(pct)}%` : '0%';
  }
}

function sinceText(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getTime() === 0) return 'Counting.';
  return `Counting since ${d.toLocaleDateString(undefined,
    { year: 'numeric', month: 'short', day: 'numeric' })}.`;
}

async function refresh() {
  let st;
  try {
    const res = await fetch('/api/stats');
    if (!res.ok) throw new Error(String(res.status));
    st = await res.json();
  } catch {
    $('numStatusText').textContent = 'The counters are not answering. Retrying.';
    $('numStatus').classList.remove('arrived');
    return;
  }
  $('numStatus').classList.add('arrived');
  $('numStatusText').textContent = 'Live. Refreshes every 15 seconds.';
  $('sinceLine').textContent = sinceText(st.since);

  countTo($('nCreated'), st.created, human);
  countTo($('nOpened'), st.opened, human);
  countTo($('nBurned'), st.burned, human);
  countTo($('nSealed'), st.sealed, humanBytes);
  $('nAnswered').textContent = human(st.answered);

  setBar('barTTL', [['ttlHour', st.ttlHour], ['ttlDay', st.ttlDay], ['ttlWeek', st.ttlWeek]]);
  setBar('barViews', [['views1', st.views1], ['views2', st.views2], ['views5', st.views5]]);
}

refresh();
setInterval(() => {
  // skip background tabs; the first visible tick catches up
  if (document.visibilityState === 'visible') refresh();
}, REFRESH_MS);
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') refresh();
});
