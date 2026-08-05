// Mesmo motivo do /cadastro: sem SSR, a página só renderiza depois de o JS
// hidratar, e o submit nativo nunca chega a disparar antes do onsubmit.
export const ssr = false;
