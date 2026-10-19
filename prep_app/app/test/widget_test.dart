import 'package:flutter_test/flutter_test.dart';
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
}
