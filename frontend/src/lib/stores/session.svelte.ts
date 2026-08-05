// Estado de sessão compartilhado entre o header, a home e as páginas
// autenticadas. É a única fonte da verdade no cliente sobre quem está logado.
import { me, sair, type SessaoResponse } from '$lib/api/auth';

export class Sessao {
	usuario = $state<SessaoResponse | null>(null);
	carregando = $state(true);

	// carregar consulta /auth/me e trata qualquer falha (401 ou rede) como
	// deslogado — o header não pode quebrar por causa da API fora do ar.
	// Se a store já foi populada (ex: pelo guard do painel), não repete a chamada.
	async carregar(): Promise<void> {
		if (this.usuario) {
			this.carregando = false;
			return;
		}
		try {
			this.usuario = await me();
		} catch {
			this.usuario = null;
		} finally {
			this.carregando = false;
		}
	}

	// definir popula a store com o usuário autenticado (usado pelo guard do
	// painel e logo depois do cadastro ou login, que já devolvem a conta).
	definir(u: SessaoResponse): void {
		this.usuario = u;
		this.carregando = false;
	}

	// limpar zera o estado local sem tocar na API.
	limpar(): void {
		this.usuario = null;
		this.carregando = false;
	}

	// encerrar sai na API e limpa o estado local mesmo se a rede falhar: manter
	// a tela dizendo "logado" depois de a pessoa clicar em sair é pior do que
	// perder a confirmação do servidor.
	async encerrar(): Promise<void> {
		try {
			await sair();
		} finally {
			this.limpar();
		}
	}
}

export const sessao = new Sessao();
