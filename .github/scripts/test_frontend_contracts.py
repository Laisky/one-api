"""Prevent dependency audits and legacy browser acceptance from becoming optional."""
import unittest

from test_workflows import THEMES, UniqueKeyLoader, commands, load_workflow
import yaml


class FrontendContractTests(unittest.TestCase):
    """FrontendContractTests protect the quality checks for all shipped themes."""

    def test_audits_cover_every_theme_and_fail_closed(self) -> None:
        """test_audits_cover_every_theme_and_fail_closed rejects optional audit gates."""
        ci = load_workflow('lint.yml')
        for theme in THEMES:
            job = ci['jobs'][f'{theme}_frontend_tests']
            audit = [step for step in job['steps'] if f'frontend-audit.mjs {theme}' in step.get('run', '')]
            self.assertEqual(len(audit), 1)
            self.assertNotIn('if', audit[0])
            self.assertNotIn('continue-on-error', audit[0])
            self.assertNotIn('||', audit[0]['run'])
            self.assertIn('frontend-audit.test.mjs', audit[0]['run'])
            self.assertIn('yarn build', commands(job))
            self.assertIn(f'{theme}_frontend_tests', ci['jobs']['required']['needs'])

    def test_both_legacy_themes_require_real_browser_acceptance(self) -> None:
        """test_both_legacy_themes_require_real_browser_acceptance prevents build-only validation."""
        ci = load_workflow('lint.yml')
        filter_step = next(step for step in ci['jobs']['changes']['steps'] if step.get('id') == 'filter')
        filters = yaml.load(filter_step['with']['filters'], Loader=UniqueKeyLoader)
        for theme in ('air', 'berry'):
            job = ci['jobs'][f'{theme}_frontend_tests']
            browser = [step for step in job['steps'] if f'legacy-browser.py {theme}' in step.get('run', '')]
            self.assertEqual(len(browser), 1)
            self.assertNotIn('if', browser[0])
            self.assertNotIn('continue-on-error', browser[0])
            self.assertIn('web/legacy/**', filters[theme])
            self.assertNotIn('--passWithNoTests', commands(job))
            self.assertNotIn('--watchAll', commands(job))


if __name__ == '__main__':
    unittest.main(verbosity=2)
