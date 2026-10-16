"""Preserve automatic dependency audits and complete optional browser qualification."""
import unittest

from test_workflows import THEMES, UniqueKeyLoader, commands, load_workflow
import yaml


class FrontendContractTests(unittest.TestCase):
    """FrontendContractTests protect the quality checks for all shipped themes."""

    def test_audits_cover_every_theme_and_fail_closed(self) -> None:
        """test_audits_cover_every_theme_and_fail_closed rejects optional audit gates."""
        ci = load_workflow('lint.yml')
        for theme in THEMES:
            job = ci['jobs'][f'{theme}_dependency_audit']
            audit = [step for step in job['steps'] if f'frontend-audit.mjs {theme}' in step.get('run', '')]
            self.assertEqual(len(audit), 1)
            self.assertNotIn('if', audit[0])
            self.assertNotIn('continue-on-error', audit[0])
            self.assertNotIn('||', audit[0]['run'])
            self.assertIn('frontend-audit.test.mjs', audit[0]['run'])
            self.assertIn(f"needs.changes.outputs.{theme} == 'true'", job['if'])
            self.assertIn('workflow_dispatch', job['if'])
            self.assertIn('yarn install --frozen-lockfile', commands(job))
            self.assertNotIn('playwright', commands(job))
            self.assertNotIn('yarn build', commands(job))
            self.assertIn(f'{theme}_dependency_audit', ci['jobs']['required']['needs'])

    def test_release_input_check_is_required_without_installing_frontend_dependencies(self) -> None:
        """test_release_input_check_is_required rejects optional or self-updating manifest checks."""
        ci = load_workflow('lint.yml')
        changes = ci['jobs']['changes']
        self.assertIn('changes', ci['jobs']['required']['needs'])
        steps = [step for step in changes['steps'] if 'browser-release-inputs.mjs check' in step.get('run', '')]
        self.assertEqual(len(steps), 1)
        step = steps[0]
        self.assertNotIn('if', step)
        self.assertNotIn('continue-on-error', step)
        self.assertNotIn('||', step['run'])
        self.assertIn('node --test .github/scripts/browser-release-inputs.test.mjs', step['run'])
        self.assertNotIn('yarn install', commands(changes))
        self.assertNotIn('browser-release-inputs.mjs add', commands(changes))
        setup = [step for step in changes['steps'] if step.get('uses', '').startswith('actions/setup-node@')]
        self.assertEqual(setup[0]['with']['node-version'], '24')
        filter_step = next(step for step in changes['steps'] if step.get('id') == 'filter')
        filters = yaml.load(filter_step['with']['filters'], Loader=UniqueKeyLoader)
        for theme in THEMES:
            self.assertIn('web/legacy/**', filters[theme])

    def test_manual_legacy_qualification_requires_real_browser_acceptance(self) -> None:
        """test_manual_legacy_qualification_requires_real_browser_acceptance prevents build-only validation."""
        ci = load_workflow('lint.yml')
        filter_step = next(step for step in ci['jobs']['changes']['steps'] if step.get('id') == 'filter')
        filters = yaml.load(filter_step['with']['filters'], Loader=UniqueKeyLoader)
        for theme in ('air', 'berry'):
            job = ci['jobs'][f'{theme}_frontend_tests']
            self.assertEqual(job['if'], "github.event_name == 'workflow_dispatch' && inputs.qualification")
            browser = [step for step in job['steps'] if f'legacy-browser.py {theme}' in step.get('run', '')]
            self.assertEqual(len(browser), 1)
            self.assertNotIn('if', browser[0])
            self.assertNotIn('continue-on-error', browser[0])
            self.assertIn('web/legacy/**', filters[theme])
            self.assertNotIn('--passWithNoTests', commands(job))
            self.assertNotIn('--watchAll', commands(job))


if __name__ == '__main__':
    unittest.main(verbosity=2)
