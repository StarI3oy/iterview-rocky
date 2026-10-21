import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import 'http_client.dart';
import 'model_catalog.dart';
import 'models.dart';

class ApiException implements Exception {
  ApiException(this.message, [this.status]);

  final String message;
  final int? status;

  @override
  String toString() => message;
}

class AuthStatus {
  const AuthStatus({required this.passwordSet, required this.authenticated});

  final bool passwordSet;
  final bool authenticated;
}

class Api {
  Api({http.Client? client, String? base})
    : client = client ?? createHttpClient(),
      base = (base ?? defaultBase()).replaceAll(RegExp(r'/+$'), '');

  final http.Client client;
  final String base;
  String? token;
  void Function()? onUnauthorized;

  static String defaultBase() {
    const fromEnv = String.fromEnvironment('API_BASE');
    if (fromEnv.isNotEmpty) return fromEnv;
    if (kIsWeb) {
      final uri = Uri.base;
      if (uri.hasScheme && uri.host.isNotEmpty) return uri.origin;
    }
    return 'http://127.0.0.1:8080';
  }

  Future<AuthStatus> authStatus() async {
    final json = await _get('/api/auth/status');
    return AuthStatus(
      passwordSet: asBool(json['password_set']),
      authenticated: asBool(json['authenticated']),
    );
  }

  Future<void> login(String password) async {
    final json = await _post('/api/auth/login', {'password': password});
    final value = json['token']?.toString() ?? '';
    token = value.isEmpty ? null : value;
  }

  Future<void> setupPassword(String password, String confirm) async {
    await _post('/api/auth/setup', {'password': password, 'confirm': confirm});
  }

  Future<void> changePassword({
    required String current,
    required String password,
    required String confirm,
  }) async {
    await _post('/api/auth/password', {
      'current_password': current,
      'password': password,
      'confirm': confirm,
    });
    token = null;
  }

  Future<void> logout() async {
    try {
      await _post('/api/auth/logout', const {});
    } finally {
      token = null;
    }
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

  Map<String, String> _headers({bool json = false}) {
    return {
      if (json) 'Content-Type': 'application/json',
      'X-Prep-Client': '1',
      if (token != null && token!.isNotEmpty) 'Authorization': 'Bearer $token',
    };
  }

  Future<ModelSettings> modelSettings() async =>
      ModelSettings.fromJson(await _get('/api/settings/model'));

  Future<ModelSettings> saveModel({
    required String model,
    required String provider,
  }) async {
    final json = await _post('/api/settings/model', {
      'model': model,
      'provider': provider,
    }, timeout: const Duration(seconds: 40));
    return ModelSettings.fromJson(json);
  }

  Future<ModelSettings> resetModel() async {
    final json = await _post(
      '/api/settings/model/reset',
      const {},
      timeout: const Duration(seconds: 20),
    );
    return ModelSettings.fromJson(json);
  }

  Future<CatalogPage> searchModels(String query) async {
    final json = await _get('/api/models', {
      'q': query,
    }, const Duration(seconds: 50));
    return CatalogPage.fromJson(json);
  }

  Future<Map<String, dynamic>> _get(
    String path, [
    Map<String, String>? query,
    Duration timeout = const Duration(seconds: 20),
  ]) {
    final uri = Uri.parse('$base$path').replace(queryParameters: query);
    return _send(path, client.get(uri, headers: _headers()), timeout: timeout);
  }

  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body, {
    Duration timeout = const Duration(seconds: 20),
  }) {
    return _send(
      path,
      client.post(
        Uri.parse('$base$path'),
        headers: _headers(json: true),
        body: jsonEncode(body),
      ),
      timeout: timeout,
    );
  }

  Future<Map<String, dynamic>> _send(
    String path,
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
        final text = message == null || message.isEmpty
            ? 'Ошибка ${res.statusCode}'
            : message;
        if (res.statusCode == 401 && text != 'неверный пароль') {
          const quiet = {
            '/api/auth/login',
            '/api/auth/setup',
            '/api/auth/status',
          };
          if (!quiet.contains(path)) {
            token = null;
            onUnauthorized?.call();
          }
        }
        throw ApiException(text, res.statusCode);
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
