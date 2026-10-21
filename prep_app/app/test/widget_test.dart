import 'package:flutter_test/flutter_test.dart';
import 'package:prep_app/model_catalog.dart';
import 'package:prep_app/models.dart';

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

  test('model search keeps provider price, privacy and benchmark', () {
    final page = CatalogPage.fromJson({
      'query': 'haiku',
      'match_count': 1,
      'truncated': false,
      'latency_needs_key': false,
      'data_policy_loaded': true,
      'results': [
        {
          'id': 'anthropic/claude-haiku',
          'name': 'Claude Haiku',
          'intelligence_index': 43.4,
          'benchmark_lines': ['Интеллект 43.4'],
          'benchmark_note': 'Artificial Analysis',
          'price_hint': 'вход от \$0.15 / 1M',
          'providers': [
            {
              'name': 'Anthropic',
              'tag': 'anthropic',
              'availability': 'доступен',
              'collects_data': true,
              'data_line': 'Собирает данные: хранит промпты 30 дн.',
              'latency_line': 'До первого токена за 30 мин: p50 650 мс',
              'pricing_lines': ['Вход: \$0.15 за 1M токенов'],
              'uptime_line': 'Доступность: 30 мин 99.5%',
            },
          ],
        },
      ],
    });
    final model = page.results.single;
    final provider = model.providers.single;
    expect(model.intelligenceIndex, 43.4);
    expect(provider.collectsData, isTrue);
    expect(provider.pricingLines.single, contains('0.15'));
    expect(provider.latencyLine, contains('650'));
    expect(provider.availability, 'доступен');
  });
}
