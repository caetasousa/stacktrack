<script lang="ts">
	import './layout.css';

	let { children } = $props();
</script>

<div class="flex min-h-screen flex-col bg-canvas font-sans text-body">
	<header class="border-b border-hairline">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
			<a href="/" class="text-sm font-semibold tracking-tight text-ink">kanbanGo</a>
			<span class="text-xs text-mute">fase 0 · fundação</span>
		</div>
	</header>

	<main class="mx-auto flex w-full max-w-5xl flex-1 flex-col px-6 py-16">
		{@render children()}
	</main>

	<footer class="border-t border-hairline">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-8 text-xs text-mute">
			<span>kanbanGo — quadro colaborativo em tempo real.</span>
			<span>Projeto de estudo · Go + SvelteKit</span>
		</div>
	</footer>
</div>
