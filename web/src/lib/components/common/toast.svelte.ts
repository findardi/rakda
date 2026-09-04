// Shared toast state — a single transient notification, app-wide. Pages call
// `showToast(...)` and render one `<Toaster />`; no per-page duplication.
type Variant = 'success' | 'error';

// An optional link, for a result that lands on another page ("see it there").
export type ToastAction = { label: string; href: string };

// Reactive holder; `.current` is the live notification (null when none).
export const store = $state<{
	current: { message: string; variant: Variant; action?: ToastAction } | null;
}>({
	current: null
});

let timer: ReturnType<typeof setTimeout>;

// A toast that carries a link stays longer: a link the reader cannot reach in
// time is worse than none.
export function showToast(message: string, variant: Variant = 'success', action?: ToastAction) {
	store.current = { message, variant, action };
	clearTimeout(timer);
	timer = setTimeout(() => (store.current = null), action ? 6000 : 4000);
}
