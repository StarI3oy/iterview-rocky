import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:prep_app/api.dart';
import 'package:prep_app/models.dart';
import 'package:prep_app/screens/import_screen.dart';
import 'package:prep_app/theme.dart';

void main() {
  test('quiz item parses options and does not require code', () {
    final item = PrepItem.fromJson({
      'id': 3,
      'item_type': 'quiz',
      'block': 'go',
      'block_title': 'Go/Concurrency',
      'title': 'Канал',
      'prompt': 'Что будет при send в закрытый канал?',
      'options': ['блок', 'паника', 'ноль', 'закрытие'],
      'correct_index': 1,
      'requires_code': false,
    });
    expect(item.id, 3);
    expect(item.options, ['блок', 'паника', 'ноль', 'закрытие']);
    expect(item.correctIndex, 1);
    expect(item.isCode, isFalse);
  });

  test('empty bar stays at zero', () {
    final bar = BarStatus.fromJson({
      'overall_percent': 0,
      'done': 0,
      'total': 0,
      'empty': true,
      'blocks': [
        {
          'id': 'go',
          'title': 'Go/Concurrency',
          'percent': 0,
          'done': 0,
          'total': 0,
        },
      ],
    });
    expect(bar.empty, isTrue);
    expect(bar.blocks.single.title, 'Go/Concurrency');
  });

  test('search hit opens the question, not the hard-concept row', () {
    final hit = SearchHit.fromJson({
      'kind': 'hard',
      'id': 9,
      'question_id': 4,
      'title': 'Канал',
      'snippet': 'паника',
    });
    expect(hit.openId, 4);
  });

  testWidgets('broken cornerstone rejects the whole list and previews it', (
    tester,
  ) async {
    final client = _ScriptClient();
    final api = Api(client: client, base: 'http://test.local');
    const payload =
        '{"questions":[{"id":8,"question":"Что такое GMP?","answer":"G, M и P"}]}';
    tester.view.physicalSize = const Size(400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: ImportScreen(
            api: api,
            pickFile: () async =>
                (bytes: utf8.encode(payload), name: 'questions.json'),
          ),
        ),
      ),
    );
    await tester.pump();

    await tester.tap(find.text('Выбрать файл'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(
      find.text('Ошибка формата в краеугольном поле — весь список не проходит'),
      findsOneWidget,
    );
    final mismatch = find.text('вопрос #4 · grade: грейд не из списка');
    expect(mismatch, findsOneWidget);
    expect(tester.widget<Text>(mismatch).style?.color, AppColors.bad);
    final row = find.text('вопрос #2 · Go — пустой ответ');
    expect(row, findsOneWidget);
    expect(tester.widget<Text>(row).style?.color, AppColors.bad);
    final okRow = find.text('вопрос #1 · Go — Как устроен slice?');
    expect(okRow, findsOneWidget);
    expect(tester.widget<Text>(okRow).style?.color, AppColors.text);
    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Записать в базу'),
          )
          .onPressed,
      isNull,
    );
    expect(client.previewPath, '/api/import/preview');
    expect(client.importBody, contains('questions.json'));
    expect(client.importClient, '1');
  });
}

class _ScriptClient extends http.BaseClient {
  String importBody = '';
  String importClient = '';
  String previewPath = '';

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final bytes = await request.finalize().toBytes();
    final path = request.url.path;
    if (path == '/api/import/preview' || path == '/api/import') {
      importBody = utf8.decode(bytes);
      importClient = request.headers['x-prep-client'] ?? '';
      previewPath = path;
      return _json(200, {
        'ok': false,
        'rejected': true,
        'imported': 0,
        'questions': 1,
        'prep_items': 0,
        'concepts': 0,
        'tracker': 0,
        'issues': [
          {
            'kind': 'question',
            'id': 4,
            'field': 'grade',
            'message': 'грейд не из списка',
          },
        ],
        'preview': [
          {
            'kind': 'question',
            'id': 1,
            'title': 'Go',
            'text': 'Как устроен slice?',
            'ok': true,
          },
          {
            'kind': 'question',
            'id': 2,
            'title': 'Go',
            'text': 'пустой ответ',
            'ok': false,
          },
        ],
      });
    }
    if (path == '/api/bar') {
      return _json(200, {
        'overall_percent': 0,
        'done': 0,
        'total': 0,
        'empty': true,
        'blocks': [
          {'id': 'go', 'title': 'Go', 'percent': 0, 'done': 0, 'total': 0},
        ],
      });
    }
    if (path == '/api/queue' || path == '/api/hard') {
      return _json(200, {'items': []});
    }
    return _json(404, {'error': 'нет маршрута'});
  }

  http.StreamedResponse _json(int status, Object body) {
    return http.StreamedResponse(
      Stream.value(utf8.encode(jsonEncode(body))),
      status,
      headers: const {'content-type': 'application/json; charset=utf-8'},
    );
  }
}
