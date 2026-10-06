#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
export ARENA_VENDOR=${ARENA_VENDOR:-/opt/arena/work/php_restcord/vendor/autoload.php}
export ARENA_COMMIT=${ARENA_COMMIT:-c3d6f8f2c13851cfd426c70a718171681552be61}
case ${1:-} in
  prepare) php -r "require getenv('ARENA_VENDOR'); if (!class_exists('RestCord\\DiscordClient')) exit(1);" ;;
  *) exec php -d display_errors=stderr "$dir/main.php" "$@" ;;
esac
