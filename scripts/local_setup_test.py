"""Check observable installation and credential-preservation behavior offline."""
import io
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock

import local_setup as setup


class LocalSetupTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="selectelsdk-setup-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.path = self.root / "private/profiles.json"

    def test_profile_roundtrip_preserves_other_records_and_permissions(self):
        secret = ' synthetic "quote" \\ slash $() `literal`\nюникод '
        setup.save_profile(self.path, "one", "100", "service-one", "synthetic-one")
        previous = setup.read_profiles(self.path)["one"]
        setup.save_profile(self.path, "two", "200", "service-two", secret)
        records = setup.read_profiles(self.path)
        self.assertEqual(previous, records["one"])
        self.assertEqual(secret, records["two"]["password"])
        self.assertEqual(0o600, stat.S_IMODE(self.path.stat().st_mode))
        self.assertEqual(0o700, stat.S_IMODE(self.path.parent.stat().st_mode))
        before = self.path.read_bytes()
        with self.assertRaises(setup.SetupError):
            setup.save_profile(self.path, "two", "200", "changed", "synthetic-changed")
        self.assertEqual(before, self.path.read_bytes())
        setup.save_profile(self.path, "two", "200", "changed", "synthetic-changed", replace=True)
        self.assertEqual(previous, setup.read_profiles(self.path)["one"])

    def test_invalid_existing_data_symlink_and_failed_replace_are_preserved(self):
        self.path.parent.mkdir(mode=0o700)
        self.path.write_text("{invalid JSON")
        with self.assertRaises(setup.SetupError):
            setup.save_profile(self.path, "one", "100", "test", "synthetic")
        self.assertEqual("{invalid JSON", self.path.read_text())
        original = self.root / "original.json"
        original.write_text('{}')
        self.path.unlink()
        self.path.symlink_to(original)
        with self.assertRaises(setup.SetupError):
            setup.save_profile(self.path, "one", "100", "test", "synthetic")
        self.assertEqual('{}', original.read_text())
        self.path.unlink()
        setup.save_profile(self.path, "one", "100", "test", "synthetic")
        before = self.path.read_bytes()
        with mock.patch.object(setup.os, "replace", side_effect=OSError("synthetic failure")):
            with self.assertRaises(OSError):
                setup.save_profile(self.path, "two", "100", "test", "synthetic")
        self.assertEqual(before, self.path.read_bytes())
        self.assertEqual([], list(self.path.parent.glob(".selectelsdk-*")))

    def test_writer_accepts_fd_input_without_secret_output(self):
        secret = ' synthetic \\" $() `literal` юникод '
        helper = Path(setup.__file__).resolve()
        result = subprocess.run(
            ["bash", "-c", 'exec 3<&0; exec python3 "$1" save-profile --path "$2" --name "$3"',
             "profile-test", str(helper), str(self.path), "one"],
            input=("100\0service-name\0" + secret + "\0").encode(),
            capture_output=True, check=True)
        self.assertNotIn(secret.encode(), result.stdout + result.stderr)
        self.assertEqual(secret, setup.read_profiles(self.path)["one"]["password"])

    def test_skill_installation_rebases_links_and_preserves_unowned_changes(self):
        source = self.root / "SDK with spaces/.agents/skills/selectelsdk"
        source.mkdir(parents=True)
        source.joinpath("references").mkdir()
        sdk_root = source.parents[2]
        sdk_root.joinpath("README.md").write_text("# SDK\n")
        source.joinpath("SKILL.md").write_text("[SDK](../../../README.md) [usage](references/usage.md)\n")
        source.joinpath("references/usage.md").write_text("[SDK](../../../../README.md#sdk)\n")
        destination = self.root / "user-skills/selectelsdk"
        metadata = {"revision": "synthetic-revision", "sdk_root": str(sdk_root)}
        setup.install_skill(source, destination, metadata)
        self.assertIn("](<" + str((sdk_root / "README.md").resolve()) + ">)", destination.joinpath("SKILL.md").read_text())
        self.assertIn("[usage](references/usage.md)", destination.joinpath("SKILL.md").read_text())
        setup.install_skill(source, destination, dict(metadata, revision="synthetic-next"))
        destination.joinpath("SKILL.md").write_text("user's changed skill")
        with self.assertRaises(setup.SetupError):
            setup.install_skill(source, destination, metadata)
        self.assertEqual("user's changed skill", destination.joinpath("SKILL.md").read_text())
        destination.joinpath("installation.json").unlink()
        with self.assertRaises(setup.SetupError):
            setup.install_skill(source, destination, metadata)

    def test_archive_rejects_escape_and_symlinks(self):
        for name, kind in [("../escape", tarfile.REGTYPE), ("link", tarfile.SYMTYPE)]:
            archive = self.root / "bad.tar"
            with tarfile.open(archive, "w") as bundle:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = "../escape"
                bundle.addfile(member)
            target = self.root / "extracted"
            target.mkdir(exist_ok=True)
            with self.assertRaises(setup.SetupError):
                setup.extract_files(archive, target)
            self.assertFalse(self.root.joinpath("escape").exists())

    def test_go_download_checks_checksum_before_extraction(self):
        item = {"filename": "go1.26.8.linux-amd64.tar.gz", "version": "go1.26.8",
                "os": "linux", "arch": "amd64", "kind": "archive", "sha256": "0" * 64, "size": 7}
        release = [{"version": "go1.26.8", "stable": True, "files": [item]}]
        with mock.patch.object(setup.platform, "system", return_value="Linux"), \
             mock.patch.object(setup.platform, "machine", return_value="x86_64"), \
             mock.patch.object(setup.urllib.request, "urlopen", side_effect=[io.BytesIO(json.dumps(release).encode()), io.BytesIO(b"invalid")]), \
             mock.patch.object(setup, "usable_go", return_value=False):
            with self.assertRaisesRegex(setup.SetupError, "Проверка архива"):
                setup.download_go(self.root, (1, 26, 0))
        self.assertFalse(self.root.joinpath("toolchains/go1.26.8").exists())

    def test_go_release_selection_uses_stable_compatible_archive(self):
        def release(ver, stable, arch="arm64"):
            return {"version": ver, "stable": stable,
                    "files": [{"version": ver, "os": "darwin", "arch": arch, "kind": "archive"}]}
        releases = [release("go1.27.2", True), release("go1.26.8", True),
                    release("go1.26.7", True), release("go1.26.9", False)]
        chosen = setup.select_go_file(releases, (1, 26, 0), "darwin", "arm64")
        self.assertEqual("go1.26.8", chosen["version"])


class InstallerIntegrationTests(unittest.TestCase):
    def test_isolated_install_rerun_and_skill_conflict_without_cloud_access(self):
        repository = Path(__file__).resolve().parents[1]
        go = shutil.which("go")
        self.assertIsNotNone(go, "Go is required for the installer integration check")
        with tempfile.TemporaryDirectory(prefix="selectelsdk-install-test-") as temporary:
            root = Path(temporary)
            fixture = root / "source"
            fixture.mkdir()
            names = subprocess.run(["git", "ls-files", "-z"], cwd=repository,
                                   check=True, capture_output=True).stdout.decode().split("\0")
            for name in filter(None, names):
                source = repository / name
                target = fixture / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
            for command in (["git", "init", "--quiet", "--initial-branch=main"], ["git", "add", "."],
                            ["git", "-c", "user.name=Setup test", "-c", "user.email=setup@example.invalid",
                             "commit", "--quiet", "-m", "Fixture"]):
                subprocess.run(command, cwd=fixture, check=True, capture_output=True)
            profiles = root / "profiles/profiles.json"
            setup.save_profile(profiles, "personal", "100", "synthetic-user", "synthetic-password")
            before = profiles.read_bytes()
            command = ["bash", str(fixture / "install.sh"), "--data-dir", str(root / "data"),
                       "--skills-dir", str(root / "skills"), "--profiles-path", str(profiles),
                       "--profile", "personal", "--skip-profile", "--go", go]
            environment = dict(os.environ, GOPROXY="off")
            fake_bin = root / "test-bin"
            fake_bin.mkdir()
            git_wrapper = fake_bin / "git"
            git_wrapper.write_text(
                '#!/usr/bin/env python3\nimport os, sys\n'
                'args = [os.environ["SETUP_TEST_REPOSITORY"] if arg == "https://github.com/festy23/selectel-sdk.git" else arg for arg in sys.argv[1:]]\n'
                'os.execv(os.environ["SETUP_TEST_GIT"], [os.environ["SETUP_TEST_GIT"]] + args)\n')
            git_wrapper.chmod(0o700)
            pipeline_environment = dict(environment, PATH=str(fake_bin) + os.pathsep + os.environ["PATH"],
                                        SETUP_TEST_REPOSITORY=str(fixture), SETUP_TEST_GIT=shutil.which("git"))
            pipeline = subprocess.run(["bash", "-s", "--"] + command[2:],
                                      input=fixture.joinpath("install.sh").read_text(),
                                      check=False, capture_output=True, text=True,
                                      cwd=root, env=pipeline_environment, timeout=180)
            self.assertEqual(0, pipeline.returncode, pipeline.stderr)
            self.assertIn("cloud_requests=0", pipeline.stdout)
            for _ in range(2):
                result = subprocess.run(command, check=True, capture_output=True, text=True,
                                        env=environment, timeout=180)
                self.assertIn("cloud_requests=0", result.stdout)
                self.assertNotIn("synthetic-password", result.stdout + result.stderr)
                self.assertEqual(before, profiles.read_bytes())
            installed = root / "skills/selectelsdk"
            metadata = json.loads(installed.joinpath("installation.json").read_text())
            self.assertTrue(Path(metadata["sdk_root"]).joinpath("go.mod").is_file())
            self.assertEqual(str(profiles), metadata["profiles_path"])
            self.assertEqual("personal", metadata["profile_name"])
            self.assertIn(str(Path(metadata["sdk_root"]) / "README.md"), installed.joinpath("SKILL.md").read_text())
            original_skill = installed.joinpath("SKILL.md").read_text()
            installed.joinpath("SKILL.md").write_text("personal customization")
            result = subprocess.run(command, capture_output=True, text=True, env=environment, timeout=30)
            self.assertNotEqual(0, result.returncode)
            self.assertEqual("personal customization", installed.joinpath("SKILL.md").read_text())
            self.assertEqual(before, profiles.read_bytes())
            installed.joinpath("SKILL.md").write_text(original_skill)
            sdk_readme = Path(metadata["sdk_root"]) / "README.md"
            sdk_readme.write_text("personal SDK customization")
            result = subprocess.run(command, capture_output=True, text=True, env=environment, timeout=30)
            self.assertNotEqual(0, result.returncode)
            self.assertEqual("personal SDK customization", sdk_readme.read_text())
            self.assertEqual(before, profiles.read_bytes())


if __name__ == "__main__":
    unittest.main()
