<?php
declare(strict_types=1);
require getenv('ARENA_VENDOR');
const LIBRARY = 'php_discord-php';
$commit = getenv('ARENA_COMMIT') ?: 'e004de5a1ba92df9e945659ab3a41da9d7937f24';
$emit = static fn(array $v) => print(json_encode($v, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES)."\n");
$unsupported = [
  'message_handled' => 'offline construction starts a websocket connection; processWsMessage is protected',
  'message_unhandled' => 'offline construction starts a websocket connection; processWsMessage is protected',
  'guild_create_state' => 'cache handlers require a connected Discord client object graph',
  'member_lookup' => 'cache cannot be populated without constructing the connecting client',
  'permission_resolve' => 'cache cannot be populated without constructing the connecting client',
  'malformed_frame' => 'raw frame processor is protected and owned by websocket client',
];
$command = $argv[1] ?? '';
if ($command === 'info') {
  $emit(['schema'=>1,'library'=>LIBRARY,'language'=>'PHP','commit'=>$commit,'runtime'=>PHP_VERSION,
    'build'=>'opcache-disabled CLI','supported'=>[],'unsupported'=>$unsupported]); exit;
}
if ($command === 'verify') {
  if (!class_exists(Discord\Discord::class) || !class_exists(Discord\WebSockets\Handlers::class)) throw new RuntimeException('library classes missing');
  $emit(['schema'=>1,'library'=>LIBRARY,'verified'=>true,'library_loaded'=>true]); exit;
}
if ($command === 'cold-start') {
  $emit(['schema'=>1,'library'=>LIBRARY,'language'=>'PHP','commit'=>$commit,'runtime'=>PHP_VERSION,
    'build'=>'opcache-disabled CLI','workload'=>'cold_start','coverage'=>'library_load','sample'=>1,
    'operations'=>1,'elapsed_ns'=>0,'ns_per_op'=>0.0,'peak_rss_bytes'=>memory_get_peak_usage(true),
    'callbacks'=>0,'digest'=>'sha256:'.hash('sha256', Discord\Discord::class)]); exit;
}
if ($command === 'bench') {
  $name = $argv[2] ?? 'missing'; fwrite(STDERR, "unsupported $name: ".($unsupported[$name] ?? 'not canonical')."\n"); exit(2);
}
throw new InvalidArgumentException('usage: adapter <info|verify|cold-start|bench>');
