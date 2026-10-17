"""Preserve automatic dependency audits and complete optional browser qualification."""
import copy
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

    def assert_release_input_contract(self, ci: dict) -> None:
        """assert_release_input_contract requires each fail-closed command and its effective Node setup, regardless of step grouping."""
        changes = ci['jobs']['changes']
        self.assertIn('changes', ci['jobs']['required']['needs'])
        steps = changes['steps']
        for command in ('node --test .github/scripts/browser-release-inputs.test.mjs',
                        'node .github/scripts/browser-release-inputs.mjs check'):
            matches = [(index, step) for index, step in enumerate(steps) if command in step.get('run', '')]
            self.assertEqual(len(matches), 1)
            index, step = matches[0]
            self.assertNotIn('if', step)
            self.assertNotIn('continue-on-error', step)
            self.assertNotIn('||', step['run'])
            setup = [previous for previous in steps[:index]
                     if previous.get('uses', '').startswith('actions/setup-node@')]
            self.assertTrue(setup, 'Release contracts require a preceding Node setup')
            self.assertRegex(str(setup[-1]['with']['node-version']), r'^v?24(?:\.(?:[0-9]+|x)){0,2}$')
            self.assertNotIn('if', setup[-1])
            self.assertNotIn('continue-on-error', setup[-1])
        self.assertNotIn('yarn install', commands(changes))
        self.assertNotIn('browser-release-inputs.mjs add', commands(changes))
        filter_step = next(step for step in steps if step.get('id') == 'filter')
        filters = yaml.load(filter_step['with']['filters'], Loader=UniqueKeyLoader)
        for theme in THEMES:
            self.assertIn('web/legacy/**', filters[theme])

    def test_release_input_check_is_required_without_installing_frontend_dependencies(self) -> None:
        """test_release_input_check_is_required rejects optional checks, self-updates and missing runtime setup."""
        self.assert_release_input_contract(load_workflow('lint.yml'))

    def test_release_input_commands_accept_equivalent_step_layouts_and_reject_bypasses(self) -> None:
        """test_release_input_commands_accept_equivalent_step_layouts preserves mandatory checks without fixing their layout."""
        ci = load_workflow('lint.yml')
        steps = ci['jobs']['changes']['steps']
        index = next(index for index, step in enumerate(steps)
                     if 'browser-release-inputs.mjs check' in step.get('run', ''))
        command = 'node --test .github/scripts/browser-release-inputs.test.mjs'
        steps[index]['run'] = steps[index]['run'].replace(command + '\n', '')
        steps.insert(index, {'name': 'Test release inputs separately', 'run': command})
        steps.insert(0, {'uses': 'actions/setup-node@v7.0.0', 'with': {'node-version': '22'}})
        self.assert_release_input_contract(ci)
        pinned = copy.deepcopy(ci)
        next(step for step in reversed(pinned['jobs']['changes']['steps'])
             if step.get('uses', '').startswith('actions/setup-node@'))['with']['node-version'] = '24.21.0'
        self.assert_release_input_contract(pinned)
        for command in ('node --test .github/scripts/browser-release-inputs.test.mjs',
                        'node .github/scripts/browser-release-inputs.mjs check'):
            for bypass in ('optional', 'ignored', 'masked', 'missing', 'runtime'):
                with self.subTest(command=command, bypass=bypass):
                    modified = copy.deepcopy(ci)
                    steps = modified['jobs']['changes']['steps']
                    index = next(index for index, step in enumerate(steps) if command in step.get('run', ''))
                    if bypass == 'optional':
                        steps[index]['if'] = 'false'
                    elif bypass == 'ignored':
                        steps[index]['continue-on-error'] = True
                    elif bypass == 'masked':
                        steps[index]['run'] += ' || true'
                    elif bypass == 'missing':
                        del steps[index]
                    else:
                        steps.insert(index, {'uses': 'actions/setup-node@v7.0.0', 'with': {'node-version': '22'}})
                    with self.assertRaises(AssertionError):
                        self.assert_release_input_contract(modified)

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
