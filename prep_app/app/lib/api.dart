import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import 'models.dart';

class ApiException implements Exception {
  ApiException(this.message, [this.status]);

  final String message;
  final int? status;

  @override
  String toString() => message;
}

class Api {
  Api({http.Client? client, String? base})
    : client = client ?? http.Client(),
      base = (base ?? defaultBase()).replaceAll(RegExp(r'/+$'), '');

  final http.Client client;
  final String base;

  static String defaultBase() {
    const fromEnv = String.fromEnvironment('API_BASE');
    if (fromEnv.isNotEmpty) return fromEnv;
    if (kIsWeb) {
      final uri = Uri.base;
      if (uri.hasScheme && uri.host.isNotEmpty) return uri.origin;
    }
    return 'http://127.0.0.1:8080';
  }

  Future<bool> aiEnabled() async {
    final json = await _get('/api/health');
    return asBool(json['ai']);
  }

  Future<BarStatus> bar() async => BarStatus.fromJson(await _get('/api/bar'));

  Future<PrepItem?> feed({
    required String mode,
    required String kind,
    int? skipId,
  }) async {
    final json = await _get('/api/feed', {
      'mode': mode,
      'kind': kind,
      'skip_id': ?skipId?.toString(),
    });
    return PrepItem.maybe(json['item']);
  }

  Future<PrepItem> item(int id) async =>
      PrepItem.fromJson(await _get('/api/items/$id'));

  Future<List<HardConcept>> hard() async {
    final json = await _get('/api/hard');
    final raw = json['items'];
    if (raw is! List) return const [];
    return raw.map((e) => HardConcept.fromJson(asMap(e))).toList();
  }

  Future<List<PrepItem>> queue() async {
    final json = await _get('/api/queue');
    final raw = json['items'];
    if (raw is! List) return const [];
    return raw.map((e) => PrepItem.fromJson(asMap(e))).toList();
  }

  Future<AnswerOutcome> answer({
    required int questionId,
    required bool passed,
    String userAnswer = '',
    int? selectedIndex,
  }) async {
    final json = await _post('/api/answer', {
      'question_id': questionId,
      'passed': passed,
      'user_answer': userAnswer,
      'selected_index': ?selectedIndex,
    });
    return AnswerOutcome.fromJson(json);
  }

  Future<ReviewVerdict> review({
    required int questionId,
    required String code,
  }) async {
    final json = await _post('/api/review', {
      'question_id': questionId,
      'code': code,
    }, timeout: const Duration(seconds: 70));
    return ReviewVerdict.fromJson(json);
  }

  Future<SearchResult> search(String query) async {
    final json = await _post('/api/search', {
      'query': query,
    }, timeout: const Duration(seconds: 70));
    return SearchResult.fromJson(json);
  }

  Future<void> enqueue(int questionId) async {
    await _post('/api/queue', {'question_id': questionId});
  }

  Future<Map<String, dynamic>> _get(String path, [Map<String, String>? query]) {
    final uri = Uri.parse('$base$path').replace(queryParameters: query);
    return _send(client.get(uri));
  }

  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body, {
    Duration timeout = const Duration(seconds: 20),
  }) {
    return _send(
      client.post(
        Uri.parse('$base$path'),
        headers: const {'Content-Type': 'application/json'},
        body: jsonEncode(body),
      ),
      timeout: timeout,
    );
  }

  Future<Map<String, dynamic>> _send(
    Future<http.Response> future, {
    Duration timeout = const Duration(seconds: 20),
  }) async {
    try {
      final res = await future.timeout(timeout);
      final dynamic decoded = res.body.isEmpty
          ? <String, dynamic>{}
          : jsonDecode(res.body);
      final map = asMap(decoded);
      if (res.statusCode >= 400) {
        final message = map['error']?.toString();
        throw ApiException(
          message == null || message.isEmpty
              ? 'Ошибка ${res.statusCode}'
              : message,
          res.statusCode,
        );
      }
      return map;
    } on ApiException {
      rethrow;
    } on TimeoutException {
      throw ApiException('Сервер не ответил вовремя');
    } catch (e) {
      throw ApiException('Нет связи с API');
    }
  }
}
