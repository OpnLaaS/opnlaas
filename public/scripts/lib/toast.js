const TOAST_ROOT_ID = "toast-root";
const DEFAULT_DURATION_MS = 5500;

const activeToasts = new Set();

function ensureToastRoot() {
    let root = document.getElementById(TOAST_ROOT_ID);
    if (root) return root;

    root = document.createElement("section");
    root.id = TOAST_ROOT_ID;
    root.className = "toast-root fixed right-4 top-20 z-[120] flex w-[min(92vw,24rem)] flex-col gap-2 pointer-events-none";
    root.setAttribute("aria-live", "polite");
    root.setAttribute("aria-atomic", "true");
    document.body.appendChild(root);
    return root;
}

function normalizeDuration(durationMs) {
    const parsed = Number(durationMs);
    if (!Number.isFinite(parsed) || parsed <= 0) {
        return DEFAULT_DURATION_MS;
    }

    return Math.max(1200, parsed);
}

function escapeHTML(value) {
    const text = `${value ?? ""}`;
    return text
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
}

export function dismissToasts(tone = "") {
    const normalizedTone = `${tone || ""}`.trim().toLowerCase();
    for (const toast of activeToasts) {
        if (!normalizedTone || toast.dataset.tone === normalizedTone) {
            toast.dismiss();
        }
    }
}

export function showToast(message, options = {}) {
    const root = ensureToastRoot();
    const tone = `${options.tone || "info"}`.trim().toLowerCase();
    const duration = normalizeDuration(options.duration_ms ?? options.durationMs);

    const toast = document.createElement("article");
    toast.className = `toast-card toast-card-${tone} pointer-events-auto rounded-lg border bg-background-secondary shadow-lg overflow-hidden`;
    toast.dataset.tone = tone;
    toast.innerHTML = `
        <div class="flex items-start gap-3 px-3 py-2.5">
            <div class="toast-dot mt-1 h-2.5 w-2.5 shrink-0 rounded-full" aria-hidden="true"></div>
            <p class="m-0 flex-1 text-sm leading-5">${escapeHTML(message)}</p>
            <button type="button" class="toast-close rounded-md px-1.5 text-sm leading-none hover:bg-background-muted" aria-label="Dismiss notification">×</button>
        </div>
        <div class="toast-progress h-1 w-full bg-background-muted/70">
            <div class="toast-progress-bar h-full"></div>
        </div>
    `;

    const closeButton = toast.querySelector(".toast-close");
    const bar = toast.querySelector(".toast-progress-bar");
    if (!closeButton || !bar) {
        return null;
    }

    let remaining = duration;
    let timeoutId = null;
    let rafId = null;
    let startedAt = performance.now();
    let removed = false;

    const updateProgress = () => {
        const clamped = Math.max(0, Math.min(remaining, duration));
        const percent = (clamped / duration) * 100;
        bar.style.width = `${percent}%`;
    };

    const stopTimers = () => {
        if (timeoutId !== null) {
            clearTimeout(timeoutId);
            timeoutId = null;
        }
        if (rafId !== null) {
            cancelAnimationFrame(rafId);
            rafId = null;
        }
    };

    const tick = () => {
        if (removed) return;
        const now = performance.now();
        const elapsed = now - startedAt;
        const left = Math.max(0, remaining - elapsed);
        bar.style.width = `${(left / duration) * 100}%`;
        if (left <= 0) return;
        rafId = requestAnimationFrame(tick);
    };

    const arm = () => {
        stopTimers();
        startedAt = performance.now();
        updateProgress();
        timeoutId = window.setTimeout(() => {
            dismiss();
        }, remaining);
        rafId = requestAnimationFrame(tick);
    };

    const pause = () => {
        if (removed) return;
        const elapsed = performance.now() - startedAt;
        remaining = Math.max(0, remaining - elapsed);
        stopTimers();
        updateProgress();
    };

    const resume = () => {
        if (removed || remaining <= 0) {
            dismiss();
            return;
        }
        arm();
    };

    const dismiss = () => {
        if (removed) return;
        removed = true;
        stopTimers();
        toast.classList.add("toast-leaving");
        window.setTimeout(() => {
            toast.remove();
            activeToasts.delete(toast);
        }, 180);
    };

    toast.dismiss = dismiss;

    closeButton.addEventListener("click", dismiss);
    toast.addEventListener("mouseenter", pause);
    toast.addEventListener("mouseleave", resume);

    root.appendChild(toast);
    activeToasts.add(toast);
    arm();
    return toast;
}

export function showErrorToast(message, options = {}) {
    return showToast(message, { ...options, tone: "error", duration_ms: options.duration_ms ?? 6500 });
}

export function showSuccessToast(message, options = {}) {
    return showToast(message, { ...options, tone: "success" });
}

export function showInfoToast(message, options = {}) {
    return showToast(message, { ...options, tone: "info" });
}

export function showWarningToast(message, options = {}) {
    return showToast(message, { ...options, tone: "warning", duration_ms: options.duration_ms ?? 7000 });
}
