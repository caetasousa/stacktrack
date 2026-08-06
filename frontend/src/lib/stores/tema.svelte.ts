// Preferência de tema. Quem aplica o tema na primeira pintura é o
// static/tema.js; esta store é a parte da aplicação: ela lê o que já está
// no <html>, deixa trocar e grava a escolha.
export type Tema = 'dark' | 'light';

export const CHAVE_TEMA = 'kanbango:tema';
export const TEMA_PADRAO: Tema = 'dark';

function ehTema(valor: unknown): valor is Tema {
	return valor === 'dark' || valor === 'light';
}

export class ControleTema {
	atual = $state<Tema>(TEMA_PADRAO);
	// persistido é false quando o localStorage não está disponível — a
	// escolha vale só nesta sessão, e a tela pode dizer isso se precisar.
	persistido = $state(true);

	// sincronizar lê o tema que o static/tema.js já aplicou no documento.
	// A store nunca decide o tema inicial sozinha: se decidisse, poderia
	// discordar do que já está pintado na tela.
	sincronizar(): void {
		if (typeof document === 'undefined') return;
		const aplicado = document.documentElement.getAttribute('data-theme');
		this.atual = ehTema(aplicado) ? aplicado : TEMA_PADRAO;
	}

	definir(tema: Tema): void {
		this.atual = tema;
		if (typeof document === 'undefined') return;

		const raiz = document.documentElement;
		raiz.setAttribute('data-theme', tema);

		try {
			localStorage.setItem(CHAVE_TEMA, tema);
			this.persistido = true;
		} catch {
			// localStorage indisponível: mantém a escolha só nesta sessão
			this.persistido = false;
		}
	}

	alternar(): void {
		this.definir(this.atual === 'dark' ? 'light' : 'dark');
	}
}

export const tema = new ControleTema();
