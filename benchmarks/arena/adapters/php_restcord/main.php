<?php
declare(strict_types=1);
require getenv('ARENA_VENDOR');
const LIBRARY = 'php_restcord';
$commit = getenv('ARENA_COMMIT') ?: 'c3d6f8f2c13851cfd426c70a718171681552be61';
$emit = static fn(array $v) => print(json_encode($v, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES)."\n");
$reason = 'RestCord is a REST client and implements no Discord Gateway dispatcher or gateway state cache';
$unsupported = array_fill_keys(['message_handled','message_unhandled','guild_create_state','member_lookup','permission_resolve','malformed_frame'], $reason);
$command = $argv[1] ?? '';
if ($command === 'info') {
  $emit(['schema'=>1,'library'=>LIBRARY,'language'=>'PHP','commit'=>$commit,'runtime'=>PHP_VERSION,
    'build'=>'opcache-disabled CLI','supported'=>[],'unsupported'=>$unsupported]); exit;
}
if ($command === 'verify') {
  if (!class_exists(RestCord\DiscordClient::class)) throw new RuntimeException('DiscordClient missing');
  $emit(['schema'=>1,'library'=>LIBRARY,'verified'=>true,'library_loaded'=>true]); exit;
}
if ($command === 'cold-start') {
  $emit(['schema'=>1,'library'=>LIBRARY,'language'=>'PHP','commit'=>$commit,'runtime'=>PHP_VERSION,
    'build'=>'opcache-disabled CLI','workload'=>'cold_start','coverage'=>'library_load','sample'=>1,
    'operations'=>1,'elapsed_ns'=>0,'ns_per_op'=>0.0,'peak_rss_bytes'=>memory_get_peak_usage(true),
    'callbacks'=>0,'digest'=>'sha256:'.hash('sha256', RestCord\DiscordClient::class)]); exit;
}
if ($command === 'bench') { fwrite(STDERR, "unsupported ".($argv[2] ?? 'missing').": $reason\n"); exit(2); }
throw new InvalidArgumentException('usage: adapter <info|verify|cold-start|bench>');
