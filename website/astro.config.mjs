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
			social: {
				gitlab: 'https://git.f4mily.net/goloom',
			},
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
					autogenerate: { directory: 'docs/getting-started' },
				},
				{
					label: 'Guides',
					autogenerate: { directory: 'docs/guides' },
				},
				{
					label: 'API',
					link: '/docs/api/',
				},
				{
					label: 'Administration',
					autogenerate: { directory: 'docs/admin' },
				},
				{
					label: 'Migrations',
					autogenerate: { directory: 'docs/migrations' },
				},
			],
		}),
	],
});
