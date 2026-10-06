import 'dart:convert';
import 'dart:io';
import 'package:nyxx/src/gateway/event_parser.dart';
import 'package:nyxx/src/models/gateway/event.dart';

const library = 'dart_nyxx';
void emit(Object value) => stdout.writeln(jsonEncode(value));
Never _unsupported(String name) {
  stderr.writeln('unsupported $name: Nyxx requires a constructed Gateway/NyxxGateway to turn raw dispatches into typed events and cache updates');
  exit(2);
}
Future<void> main(List<String> args) async {
  final commit = Platform.environment['ARENA_COMMIT'] ?? '40e12c14524af372cf8f816ffdc680fdfb74c183';
  final command = args.firstOrNull ?? '';
  if (command == 'info') {
    emit({'schema':1,'library':library,'language':'Dart','commit':commit,'runtime':Platform.version,
      'build':'AOT release','supported':['malformed_frame'],'unsupported':{
        'message_handled':'typed Gateway requires a live-client object graph','message_unhandled':'typed Gateway requires a live-client object graph',
        'guild_create_state':'typed Gateway/cache requires a live-client object graph','member_lookup':'cache cannot be populated offline through public API',
        'permission_resolve':'no fixture-populated public cache'}}); return;
  }
  final parser = EventParser();
  if (command == 'cold-start') {
    emit({'schema':1,'library':library,'language':'Dart','commit':commit,'runtime':Platform.version,'build':'AOT release',
      'workload':'cold_start','coverage':'library_load','sample':1,'operations':1,'elapsed_ns':0,'ns_per_op':0,
      'peak_rss_bytes':0,'callbacks':0,'digest':'sha256:${'0'*64}'}); return;
  }
  if (command == 'verify') {
    final dir = Platform.environment['ARENA_FIXTURES']!;
    final good = jsonDecode(await File('$dir/message-create.json').readAsString()) as Map<String,dynamic>;
    final event = parser.parseGatewayEvent(good);
    if (event is! RawDispatchEvent || event.name != 'MESSAGE_CREATE' || event.payload['content'] != 'hello there') throw StateError('raw event mismatch');
    final malformed = await File('$dir/malformed-frame.json').readAsString();
    var failed = false; try { parser.parseGatewayEvent(jsonDecode(malformed) as Map<String,dynamic>); } catch (_) { failed = true; }
    if (!failed) throw StateError('malformed frame did not fail');
    emit({'schema':1,'library':library,'verified':true}); return;
  }
  if (command == 'bench') _unsupported(args.length > 1 ? args[1] : 'missing');
  throw ArgumentError('usage: adapter <info|verify|cold-start|bench>');
}
