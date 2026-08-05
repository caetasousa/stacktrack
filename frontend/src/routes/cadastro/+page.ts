// Sem SSR, o SvelteKit só renderiza a página depois de o JS hidratar,
// eliminando a janela em que um clique dispara o submit nativo do form antes
// de o onsubmit ser anexado — o que recarregaria a página com as credenciais
// na query string.
export const ssr = false;
