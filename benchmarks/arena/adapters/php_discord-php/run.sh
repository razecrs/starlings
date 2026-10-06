#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
export ARENA_VENDOR=${ARENA_VENDOR:-/opt/arena/work/php_discord-php/vendor/autoload.php}
export ARENA_COMMIT=${ARENA_COMMIT:-e004de5a1ba92df9e945659ab3a41da9d7937f24}
case ${1:-} in
  prepare) php -r "require getenv('ARENA_VENDOR'); if (!class_exists('Discord\\Discord')) exit(1);" ;;
  *) exec php -d display_errors=stderr "$dir/main.php" "$@" ;;
esac
