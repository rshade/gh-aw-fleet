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
			plugins: [
				// Reject relative links (./foo/): under a base path they resolve
				// incorrectly, and this validator errors on them by default.
				// Internal links must be base-prefixed root-relative, e.g.
				// /gh-aw-fleet/foo/ (see docs/README.md → Content link convention).
				starlightLinksValidator({ errorOnRelativeLinks: true }),
			],
			sidebar: [
				{
					label: 'Tutorials',
					items: [
						{ label: 'Getting Started', slug: 'getting-started' },
					],
				},
				{
					label: 'How-to guides',
					items: [
						{ label: 'Install', slug: 'install' },
						{ label: 'Recover from a gpg signing failure', slug: 'recover-from-gpg-failure' },
						{ label: 'Gate CI on fleet drift', slug: 'gate-ci-on-drift' },
						{ label: 'Resume an interrupted apply', slug: 'resume-interrupted-apply' },
						{ label: 'Find your top credit burners', slug: 'find-top-credit-burners' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'CLI reference', slug: 'cli' },
						{ label: 'Configuration', slug: 'configuration' },
						{ label: 'Fleet Overview', slug: 'overview' },
					],
				},
				{
					label: 'Explanation',
					items: [
						{ label: 'Reconcile workflow', slug: 'reconcile' },
						{ label: 'Consumption and FinOps', slug: 'consumption' },
						{ label: 'Roadmap', slug: 'roadmap' },
					],
				},
			],
		}),
		sitemap(),
	],
});
