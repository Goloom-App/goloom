// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://goloom-app.github.io',
	base: '/goloom/',
	integrations: [
		starlight({
			title: 'goloom',
			tagline: 'Lightweight social planning for teams and AI agents',
			logo: {
				src: './public/favicon.svg',
				alt: 'goloom',
			},
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/Goloom-App/goloom',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/Goloom-App/goloom/edit/main/website/src/content/docs/',
			},
			customCss: ['./src/styles/theme.css'],
			components: {
				Footer: './src/components/Footer.astro',
			},
			sidebar: [
				{
					label: 'Getting Started',
					items: [{ autogenerate: { directory: 'getting-started' } }],
				},
				{
					label: 'Guides',
					items: [{ autogenerate: { directory: 'guides' } }],
				},
				{
					label: 'Administration',
					items: [{ autogenerate: { directory: 'admin' } }],
				},
				{
					label: 'Migrations',
					items: [{ autogenerate: { directory: 'migrations' } }],
				},
				{
					label: 'API Reference',
					link: '/api/',
					attrs: { target: '_self' },
					badge: { text: 'OpenAPI', variant: 'tip' },
				},
			],
		}),
	],
});
