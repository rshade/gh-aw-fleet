// @ts-check
import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';

// https://astro.build/config
export default defineConfig({
	site: 'https://rshade.github.io',
	base: '/gh-aw-fleet',
	integrations: [
		starlight({
			title: 'gh-aw-fleet',
			customCss: ['./src/styles/theme-bridge.css'],
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/rshade/gh-aw-fleet',
				},
			],
			plugins: [starlightLinksValidator()],
			sidebar: [
				{
					label: 'Guide',
					items: [
						{ label: 'Install', slug: 'install' },
						{ label: 'Configuration', slug: 'configuration' },
						{ label: 'Reconcile Workflow', slug: 'reconcile' },
						{ label: 'Consumption and FinOps', slug: 'consumption' },
						{ label: 'Roadmap', slug: 'roadmap' },
					],
				},
			],
		}),
		sitemap(),
	],
});
