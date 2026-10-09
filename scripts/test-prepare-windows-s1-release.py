#!/usr/bin/env python3
"""Focused safety checks for the published-product S1 driver adaptation."""
import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    'prepare', pathlib.Path(__file__).with_name('prepare-windows-s1-release.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
SOURCE = '''run shell-build bashy go build -o bin/bash.exe ./cmd/bash
run product-install bashy go build -o "$BASHY_BIN" ./cmd/bashy
run sharp-all bashsharp go test -json ./... -count=1 -timeout=10m
run lower-validate bashsharp-tests "$BASHY_BIN" tools/lowering/validate.sh
run lower-differential bashsharp-tests ruby tools/lowering/differential.rb
run decorators bashsharp-tests ruby tools/decorators/acceptance.rb
run agentic bashsharp-tests ruby tools/agentic/acceptance.rb
run polyglot bashsharp-tests "$BASHY_BIN" tools/polyglot-gate.sh
'''


class ReleaseDriverTest(unittest.TestCase):
    def test_product_builds_removed_and_other_gates_preserved(self):
        result = module.prepare(SOURCE)
        self.assertNotIn('go build', result)
        self.assertFalse(any(line.startswith('run lower-validate ') for line in result.splitlines()))
        self.assertIn('cp "$ROOT/product/bash.exe" "$BASH_ENGINE_BIN" || exit 2', result)
        self.assertIn('cp "$ROOT/product/bashy.exe" "$BASHY_BIN" || exit 2', result)
        for line in SOURCE.splitlines():
            if line.startswith(('run sharp-all ', 'run polyglot ')):
                self.assertIn(line + '\n', result)
        self.assertEqual(result.count('# D11 policy exclusion'), 4)
        self.assertFalse(any(' ruby ' in line for line in result.splitlines()
                             if not line.startswith('#')))

    def test_refuses_unrecognized_or_duplicate_product_steps(self):
        for source in (SOURCE.replace('./cmd/bash\n', './cmd/other\n'),
                       SOURCE + SOURCE.splitlines()[0] + '\n'):
            with self.assertRaises(ValueError):
                module.prepare(source)

    def test_refuses_changed_ruby_inventory(self):
        with self.assertRaises(ValueError):
            module.prepare(SOURCE.replace('run agentic ', 'run another '))


if __name__ == '__main__':
    unittest.main()
