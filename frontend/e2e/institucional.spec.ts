// O que o sistema mostra a quem chega sem conta, agora que ele é um serviço
// aberto a empresas, e não mais um projeto de estudo: textos legais alcançáveis,
// a página inicial falando de quem usa, e erro com caminho de volta.
//
// Os cenários estão em CENARIOS.md, área I.
import { test, expect } from '@playwright/test';

test('[I1] os textos legais estão no rodapé e no cadastro', async ({ page }) => {
	await page.goto('/');
	const legal = page.getByRole('navigation', { name: 'Informações legais' });
	await legal.getByRole('link', { name: 'Política de privacidade' }).click();
	await expect(page.getByRole('heading', { name: 'Política de Privacidade' })).toBeVisible();
	await expect(page.getByText('Lei nº 13.709/2018')).toBeVisible();

	await page.goto('/cadastro');
	await page.getByRole('main').getByRole('link', { name: 'Termos de Uso' }).click();
	await expect(page.getByRole('heading', { name: 'Termos de Uso' })).toBeVisible();
});

test('[I2] a página inicial fala do produto, e não do projeto de estudo', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
	for (const vestigio of ['Código aberto', 'GitHub', 'Argon2id', 'HttpOnly', 'Grátis']) {
		await expect(page.getByText(vestigio)).toHaveCount(0);
	}
	await expect(page.getByText('Segurança e privacidade')).toBeVisible();
});

test('[I3] um endereço que não existe mostra uma página de erro com saída', async ({ page }) => {
	await page.goto('/endereco-que-nao-existe');
	await expect(page.getByRole('heading', { name: 'Página não encontrada' })).toBeVisible();
	// Sem sessão, a saída é o início — e não um painel que mandaria para o login.
	await expect(page.getByRole('link', { name: 'Ir para o início' })).toBeVisible();
});

test('[I4] a página inicial e os termos falam com a empresa que quer aderir', async ({ page }) => {
	await page.goto('/');
	const empresas = page.getByRole('region', { name: 'Para empresas' });
	await expect(empresas.getByText('A equipe entra sem convite um a um')).toBeVisible();
	await expect(empresas.getByText('100 pessoas por quadro', { exact: false })).toBeVisible();
	await expect(page.getByText('instituição', { exact: false })).toHaveCount(0);

	await page.goto('/termos');
	await expect(page.getByRole('heading', { name: 'Uso por empresas' })).toBeVisible();
	await expect(page.getByText('instituição', { exact: false })).toHaveCount(0);
	await page.goto('/privacidade');
	await expect(page.getByRole('heading', { name: 'Quem é responsável pelos dados' })).toBeVisible();
	await expect(page.getByText('instituição', { exact: false })).toHaveCount(0);
});
