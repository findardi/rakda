import type { DownloadJobData } from '$lib/types/content';

const POLL_MS = 4000;

class DownloadJobQueue {
	jobs = $state<DownloadJobData[]>([]);
	panelOpen = $state(true);

	#workspaceId = $state<string | null>(null);
	#dismissed = $state<string[]>([]);
	#timer: ReturnType<typeof setTimeout> | null = null;

	get visible(): DownloadJobData[] {
		return this.jobs.filter((job) => !this.#dismissed.includes(job.id));
	}

	get pending(): number {
		return this.visible.filter((job) => job.status === 'pending').length;
	}

	bind(workspaceId: string): void {
		if (this.#workspaceId === workspaceId) return;

		this.#workspaceId = workspaceId;
		this.jobs = [];
		this.#dismissed = [];
		void this.refresh();
	}

	track(jobId: string): void {
		this.#dismissed = this.#dismissed.filter((id) => id !== jobId);
		this.panelOpen = true;
		void this.refresh();
	}

	dismiss(jobId: string): void {
		this.#dismissed = [...this.#dismissed, jobId];
		if (this.pending === 0) this.#stop();
	}

	href(job: DownloadJobData): string {
		const q = new URLSearchParams({ workspaceId: this.#workspaceId ?? '' });
		return `/api/content/download-jobs/${job.id}/download?${q}`;
	}

	async refresh(): Promise<void> {
		const workspaceId = this.#workspaceId;
		if (!workspaceId) return;

		try {
			const res = await fetch(
				`/api/content/download-jobs?workspaceId=${encodeURIComponent(workspaceId)}`
			);
			if (res.ok) this.jobs = (await res.json()) as DownloadJobData[];
		} catch {
			// Jaringan putus bukan kegagalan job: polling berikutnya mencoba lagi.
		}

		this.#schedule();
	}

	#schedule(): void {
		this.#stop();
		if (this.pending === 0) return;
		this.#timer = setTimeout(() => void this.refresh(), POLL_MS);
	}

	#stop(): void {
		if (this.#timer !== null) {
			clearTimeout(this.#timer);
			this.#timer = null;
		}
	}
}

export const downloadJobs = new DownloadJobQueue();
