// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://git.f4mily.net/goloom',
	integrations: [
		starlight({
			title: 'goloom',
			tagline: 'Lightweight social planning for teams and AI agents',
			logo: {
				src: './public/favicon.svg',
			},
			social: [
				{
					icon: 'gitlab',
					label: 'GitLab',
					href: 'https://git.f4mily.net/goloom',
				},
			],
			editLink: {
				baseUrl:
					'https://git.f4mily.net/goloom/edit/main/website/src/content/docs/',
			},
			customCss: ['./src/styles/custom.css'],
			components: {
				Footer: './src/components/Footer.astro',
			},
			sidebar: [
				{
					label: 'Getting Started',
					items: [{ autogenerate: { directory: 'docs/getting-started' } }],
				},
				{
					label: 'Guides',
					items: [{ autogenerate: { directory: 'docs/guides' } }],
				},
				{
					label: 'API',
					link: '/docs/api/',
				},
				{
					label: 'Administration',
					items: [{ autogenerate: { directory: 'docs/admin' } }],
				},
				{
					label: 'Migrations',
					items: [{ autogenerate: { directory: 'docs/migrations' } }],
				},
			],
		}),
	],
});
