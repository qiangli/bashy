import unittest

from tests.helpers import TempBashy
import common


class ContractTest(unittest.TestCase):
    def test_step_labels_are_bashy_words(self):
        self.assertEqual(common.step_labels(2, 20, "student", "Work block"),
                         ["step-02", "timebox-20m", "role-student", "phase-workblock"])

    def test_validate_draft_rejects_missing_steps(self):
        with self.assertRaises(SystemExit):
            common.validate_draft({"version": 1, "title": "t", "goal": "g", "minutes": 10, "steps": []})


class DispatchTest(TempBashy):
    def test_help_and_skill(self):
        self.assertIn("Usage: tutor", self.tutor("--help").stdout)
        self.assertIn("name: tutor", self.tutor("--skill").stdout)


if __name__ == "__main__":
    unittest.main()
