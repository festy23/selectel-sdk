#!/usr/bin/env python3
"""Local installation and private profile storage; no credentials in argv/logs."""
import argparse
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

MAX_PROFILE_BYTES = 1024 * 1024
INSTALLER = "festy23/selectel-sdk"


class SetupError(Exception):
    pass


def read_profiles(path):
    if path.is_symlink():
        raise SetupError("Файл профилей не должен быть символической ссылкой.")
    if not path.exists():
        return {}
    with path.open("rb") as stream:
        payload = stream.read(MAX_PROFILE_BYTES + 1)
    if len(payload) > MAX_PROFILE_BYTES:
        raise SetupError("Файл профилей превышает лимит SDK в 1 MiB.")
    try:
        records = json.loads(payload)
    except (ValueError, UnicodeError):
        raise SetupError("Некорректный JSON профилей; исходный файл сохранён.") from None
    if not isinstance(records, dict) or any(not isinstance(v, dict) for v in records.values()):
        raise SetupError("Ожидается JSON object: имя профиля → credential record.")
    return records


def private_parent(path):
    path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    if path.parent.stat().st_mode & 0o077:
        raise SetupError("Каталог профилей должен иметь права 700; измените права и повторите.")


def atomic_json(path, value):
    payload = (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()
    if len(payload) > MAX_PROFILE_BYTES:
        raise SetupError("Новый файл превышает лимит; исходный файл сохранён.")
    fd, name = tempfile.mkstemp(prefix=".selectelsdk-", dir=str(path.parent))
    try:
        with os.fdopen(fd, "wb") as stream:
            os.fchmod(stream.fileno(), 0o600)
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def save_profile(path, name, account, username, password, replace=False):
    if not name or len(name) > 128 or any(c in name for c in "\n\r\0"):
        raise SetupError("Некорректное имя профиля.")
    if not re.fullmatch(r"[0-9]+", account) or not username or not password:
        raise SetupError("Нужны номер аккаунта, имя сервисного пользователя и непустой пароль.")
    private_parent(path)
    lock_path = path.parent / ".profiles.lock"
    fd = os.open(str(lock_path), os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "a") as lock:
        os.fchmod(lock.fileno(), 0o600)
        fcntl.flock(lock, fcntl.LOCK_EX)
        records = read_profiles(path)
        if name in records and not replace:
            raise SetupError("Профиль уже существует и сохранён без изменений. Для замены используйте --replace.")
        records[name] = {"account_id": account, "username": username, "password": password}
        atomic_json(path, records)


def run(command, **kwargs):
    kwargs.setdefault("timeout", 300)
    return subprocess.run(command, check=True, **kwargs)


def version(text):
    match = re.search(r"go(\d+)\.(\d+)(?:\.(\d+))?(?:\s|$)", text)
    return tuple(int(part or 0) for part in match.groups()) if match else None


def usable_go(executable, required):
    if not executable or not Path(executable).is_file():
        return False
    try:
        result = subprocess.run([str(executable), "version"], check=True,
                                capture_output=True, text=True, timeout=10,
                                env=dict(os.environ, GOTOOLCHAIN="local"))
        found = version(result.stdout)
        return found is not None and found >= required
    except (OSError, subprocess.SubprocessError):
        return False


def extract_files(archive, destination):
    """Extract regular files/directories only, into a new private directory."""
    with tarfile.open(archive) as bundle:
        members = bundle.getmembers()
        for member in members:
            target = destination / member.name
            if (Path(member.name).is_absolute() or ".." in Path(member.name).parts
                    or not (member.isfile() or member.isdir())
                    or destination.resolve() not in target.resolve().parents):
                raise SetupError("Архив содержит недопустимый путь или тип файла.")
        bundle.extractall(destination, members=members)


def select_go_file(releases, required, system, machine):
    candidates = []
    for release in releases:
        found = version(release.get("version", ""))
        if not release.get("stable") or found is None or found < required:
            continue
        for item in release.get("files", []):
            if item.get("os") == system and item.get("arch") == machine and item.get("kind") == "archive":
                candidates.append((found, item))
    preferred = [item for item in candidates if item[0][:2] == required[:2]]
    if not candidates:
        raise SetupError("На go.dev нет совместимого Go для этой системы; установите Go вручную.")
    return max(preferred or candidates, key=lambda item: item[0])[1]


def download_go(data_dir, required):
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    machine = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
    if system is None or machine is None:
        raise SetupError("Автоустановка Go поддерживает macOS/Linux amd64/arm64; установите Go вручную.")
    with urllib.request.urlopen("https://go.dev/dl/?mode=json", timeout=30) as response:
        releases = json.loads(response.read(2 * 1024 * 1024))
    item = select_go_file(releases, required, system, machine)
    filename = item["filename"]
    if not re.fullmatch(r"go[0-9.]+\.(darwin|linux)-(amd64|arm64)\.tar\.gz", filename):
        raise SetupError("Некорректное имя официального архива Go.")
    destination = data_dir / "toolchains" / item["version"]
    executable = destination / "go/bin/go"
    if usable_go(executable, required):
        return executable
    if destination.exists() or destination.is_symlink():
        raise SetupError("Каталог локального Go уже существует, но не работает; выберите другой --data-dir.")
    destination.parent.mkdir(parents=True, exist_ok=True)
    print("Загрузка локального Go с go.dev и проверка SHA-256…", flush=True)
    with tempfile.TemporaryDirectory(prefix=".go-", dir=str(destination.parent)) as temporary:
        temporary = Path(temporary)
        archive = temporary / "go.tar.gz"
        digest = hashlib.sha256()
        size = 0
        with urllib.request.urlopen("https://go.dev/dl/" + filename, timeout=30) as response, archive.open("wb") as stream:
            while True:
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                size += len(chunk)
                if size > 300 * 1024 * 1024:
                    raise SetupError("Архив Go превышает допустимый размер.")
                digest.update(chunk)
                stream.write(chunk)
        if digest.hexdigest() != item["sha256"] or size != item["size"]:
            raise SetupError("Проверка архива Go не прошла; установка остановлена.")
        extracted = temporary / "tree"
        extracted.mkdir()
        extract_files(archive, extracted)
        if not usable_go(extracted / "go/bin/go", required):
            raise SetupError("Загруженный Go не запускается.")
        os.rename(extracted, destination)
    return executable


def file_hashes(directory):
    result = {}
    for path in sorted(directory.rglob("*")):
        if path.is_symlink():
            raise SetupError("Установленные файлы заменены ссылками; они сохранены без изменений.")
        if path.is_file() and path != directory / "installation.json":
            result[str(path.relative_to(directory))] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def check_owned_skill(destination):
    if destination.is_symlink():
        raise SetupError("Каталог скилла — ссылка; выберите другой --skills-dir.")
    if not destination.exists():
        return
    try:
        metadata = json.loads((destination / "installation.json").read_text())
    except (OSError, ValueError):
        raise SetupError("Скилл selectelsdk уже существует; он сохранён. Выберите другой --skills-dir.") from None
    if not isinstance(metadata, dict) or metadata.get("installer") != INSTALLER or metadata.get("files") != file_hashes(destination):
        raise SetupError("Существующий скилл изменён пользователем; он сохранён. Выберите другой --skills-dir.")


def install_skill(source, destination, metadata):
    check_owned_skill(destination)
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".selectelsdk-", dir=str(destination.parent)) as temporary:
        staging = Path(temporary) / "skill"
        shutil.copytree(source, staging)
        for document in staging.rglob("*.md"):
            relative = document.relative_to(staging)
            def resolve_link(match):
                link = match.group(1)
                if "://" in link or link.startswith("#"):
                    return match.group(0)
                path, separator, anchor = link.partition("#")
                target = (source / relative).parent.joinpath(path).resolve()
                if target == source.resolve() or source.resolve() in target.parents:
                    return match.group(0)
                if not target.exists():
                    raise SetupError("Ссылка скилла указывает на отсутствующий файл.")
                return "](<" + str(target) + (separator + anchor if separator else "") + ">)"
            document.write_text(re.sub(r"\]\(([^)]+)\)", resolve_link, document.read_text()))
        metadata = dict(metadata, installer=INSTALLER, files=file_hashes(staging))
        atomic_json(staging / "installation.json", metadata)
        backup = Path(temporary) / "previous"
        if destination.exists():
            os.rename(destination, backup)
        try:
            os.rename(staging, destination)
        except OSError:
            if backup.exists():
                os.rename(backup, destination)
            raise


def install(args):
    if sys.version_info < (3, 9):
        raise SetupError("Требуется Python 3.9 или новее.")
    data_dir = args.data_dir.expanduser().absolute()
    skills_dir = args.skills_dir.expanduser().absolute()
    profiles_path = args.profiles_path.expanduser().absolute()
    source = args.source.resolve()
    destination = skills_dir / "selectelsdk"
    check_owned_skill(destination)
    if not args.skip_profile:
        try:
            terminal = open("/dev/tty", "r")
        except OSError:
            raise SetupError("Для ввода профиля нужен терминал. Для установки без ввода используйте --skip-profile.") from None
    else:
        terminal = contextlib.nullcontext()
    with terminal:
        revision = run(["git", "-C", str(source), "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip()
        if subprocess.run(["git", "-C", str(source), "diff", "--quiet", "HEAD"], check=False).returncode:
            raise SetupError("Локальный checkout изменён; сохраните изменения в commit перед установкой.")
        sdk_root = data_dir / "sdk" / revision
        sdk_root.parent.mkdir(parents=True, exist_ok=True)
        if sdk_root.is_symlink():
            raise SetupError("Каталог SDK не должен быть ссылкой.")
        if not sdk_root.exists():
            with tempfile.TemporaryDirectory(prefix=".sdk-", dir=str(sdk_root.parent)) as temporary:
                archive = Path(temporary) / "sdk.tar"
                with archive.open("wb") as stream:
                    run(["git", "-C", str(source), "archive", revision], stdout=stream)
                tree = Path(temporary) / "tree"
                tree.mkdir()
                extract_files(archive, tree)
                atomic_json(tree / "installation.json", {"installer": INSTALLER, "revision": revision, "files": file_hashes(tree)})
                os.rename(tree, sdk_root)
        else:
            try:
                record = json.loads((sdk_root / "installation.json").read_text())
            except (OSError, ValueError):
                raise SetupError("Каталог версии SDK уже существует и сохранён; выберите другой --data-dir.") from None
            if not isinstance(record, dict) or record.get("installer") != INSTALLER or record.get("revision") != revision or record.get("files") != file_hashes(sdk_root):
                raise SetupError("Установленный SDK изменён; он сохранён. Выберите другой --data-dir.")
        required = version("go" + re.search(r"^go\s+(\S+)", (sdk_root / "go.mod").read_text(), re.M).group(1))
        candidate = args.go or shutil.which("go")
        if args.go and not usable_go(args.go, required):
            raise SetupError("Go, заданный через --go, не запускается или старше версии в go.mod.")
        go = Path(candidate).absolute() if usable_go(candidate, required) else download_go(data_dir, required)
        environment = dict(os.environ, GOTOOLCHAIN="local")
        environment.pop("GOROOT", None)
        print("SDK: загрузка зависимостей и проверка каталога без доступа к облаку…", flush=True)
        run([str(go), "mod", "download"], cwd=sdk_root, env=environment)
        run([str(go), "run", "./examples/discover"], cwd=sdk_root, env=environment)
        metadata = {"sdk_root": str(sdk_root), "go_executable": str(go), "revision": revision,
                    "profiles_path": str(profiles_path), "profile_name": args.profile}
        install_skill(sdk_root / ".agents/skills/selectelsdk", destination, metadata)
        print("Скилл установлен: " + str(destination), flush=True)
        if not args.skip_profile:
            run(["bash", "+x", "+v", str(sdk_root / "scripts/setup-profile.sh"),
                 str(profiles_path), args.profile], stdin=terminal, timeout=3600)
            selected = read_profiles(profiles_path).get(args.profile, {})
            account = selected.get("account_id")
            if not isinstance(account, str) or not account:
                raise SetupError("В выбранном профиле нет account_id; профиль сохранён без изменений.")
            command = [str(go), "run", "./examples/check-profile", "--profiles", str(profiles_path),
                       "--profile", args.profile, "--account", account]
            if selected.get("project_id"):
                command.extend(["--project", selected["project_id"]])
            print("Проверка IAM через SDK (без создания облачных ресурсов)…", flush=True)
            run(command, cwd=sdk_root, env=environment)
        print("SDK: " + str(sdk_root))
        print("Профили: " + str(profiles_path))
        print("Профиль: " + args.profile)
        if args.skip_profile:
            print("Ввод профиля и проверка IAM пропущены (--skip-profile).")
        print("В следующем запросе к агенту используйте $selectelsdk и имя этого профиля.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    setup = commands.add_parser("install")
    setup.add_argument("--source", type=Path, required=True)
    setup.add_argument("--data-dir", type=Path, default=Path.home() / ".local/share/selectelsdk")
    setup.add_argument("--skills-dir", type=Path, default=Path.home() / ".agents/skills")
    setup.add_argument("--profiles-path", type=Path, default=Path.home() / ".config/selectelsdk/profiles.json")
    setup.add_argument("--profile", default="default")
    setup.add_argument("--skip-profile", action="store_true")
    setup.add_argument("--go", help="Explicit existing Go executable; otherwise use PATH or a private download")
    writer = commands.add_parser("save-profile")
    writer.add_argument("--path", type=Path, required=True)
    writer.add_argument("--name", required=True)
    writer.add_argument("--replace", action="store_true")
    exists = commands.add_parser("profile-exists")
    exists.add_argument("--path", type=Path, required=True)
    exists.add_argument("--name", required=True)
    args = parser.parse_args()
    try:
        if args.command == "install":
            install(args)
        elif args.command == "profile-exists":
            return 0 if args.name in read_profiles(args.path) else 1
        else:
            with os.fdopen(3, "rb") as stream:
                payload = stream.read(MAX_PROFILE_BYTES + 1)
            fields = payload.split(b"\0")
            if len(payload) > MAX_PROFILE_BYTES or len(fields) != 4 or fields[-1] != b"":
                raise SetupError("Некорректный ввод; профиль не изменён.")
            save_profile(args.path, args.name, *(field.decode("utf-8") for field in fields[:3]), replace=args.replace)
            print("Профиль сохранён; остальные профили сохранены без изменений.")
    except SetupError as error:
        print(str(error), file=sys.stderr)
        return 2 if args.command == "profile-exists" else 1
    except (OSError, ValueError, subprocess.SubprocessError):
        print("Установка не завершена. Проверьте доступ к файлам, сети и вывод предыдущего шага; credentials не выводятся.", file=sys.stderr)
        return 2 if args.command == "profile-exists" else 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
