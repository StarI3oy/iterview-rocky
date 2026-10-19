import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api.dart';
import '../models.dart';
import '../session.dart';
import '../theme.dart';
import '../widgets/code_editor.dart';

class TrainerScreen extends StatefulWidget {
  const TrainerScreen({super.key, required this.api, required this.session});

  final Api api;
  final Session session;

  @override
  State<TrainerScreen> createState() => _TrainerScreenState();
}

class _TrainerScreenState extends State<TrainerScreen> {
  PrepItem? _item;
  String? _error;
  String? _empty;
  bool _loading = true;
  bool _busy = false;
  bool _revealed = false;
  int? _selected;
  AnswerOutcome? _outcome;
  ReviewVerdict? _verdict;
  String? _reviewError;
  String _code = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
      _empty = null;
    });
    try {
      final focus = widget.session.focusId;
      final PrepItem? item;
      if (focus != null) {
        item = await widget.api.item(focus);
      } else {
        item = await widget.api.feed(
          mode: widget.session.mode,
          kind: widget.session.kind,
        );
      }
      if (!mounted) return;
      setState(() {
        _apply(item);
        if (item == null) _empty = _emptyText();
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    }
  }

  void _apply(PrepItem? item) {
    _item = item;
    _revealed = false;
    _selected = null;
    _outcome = null;
    _verdict = null;
    _reviewError = null;
    _code = item?.starterCode ?? '';
    _busy = false;
  }

  String _emptyText() {
    switch (widget.session.kind) {
      case 'hard':
        return 'Провалов пока нет. Они появятся, если отметить «Поплыл» или не сдать ревью.';
      case 'concept':
        return 'Разобранных концептов в базе нет.';
      case 'code':
      case 'patterns':
        return 'Кодовых паттернов в базе нет.';
      case 'library':
        return 'В базе вопросов пусто.';
      default:
        return 'В этом треке карточек нет.';
    }
  }

  Future<void> _next({bool oral = false}) async {
    if (oral) {
      widget.session.openSprint();
      return;
    }
    final current = _item;
    final mode = current?.isCode == true ? 'web' : widget.session.mode;
    final kind = current?.isCode == true ? 'code' : widget.session.kind;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final item = await widget.api.feed(
        mode: mode,
        kind: kind,
        skipId: current?.id,
      );
      if (!mounted) return;
      setState(() {
        _apply(item);
        _empty = item == null ? 'В этом проходе карточки закончились.' : null;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    }
  }

  Future<void> _answerQuiz(int index) async {
    final item = _item;
    if (item == null || _outcome != null || _busy) return;
    setState(() => _busy = true);
    try {
      final outcome = await widget.api.answer(
        questionId: item.id,
        passed: false,
        userAnswer: index < item.options.length
            ? item.options[index]
            : '$index',
        selectedIndex: index,
      );
      if (!mounted) return;
      setState(() {
        _selected = index;
        _outcome = outcome;
        _busy = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _busy = false);
      _toast(e.message);
    }
  }

  Future<void> _answerCard(bool passed) async {
    final item = _item;
    if (item == null || _busy) return;
    setState(() => _busy = true);
    try {
      final outcome = await widget.api.answer(
        questionId: item.id,
        passed: passed,
        userAnswer: passed ? 'четко' : 'поплыл',
      );
      if (!mounted) return;
      setState(() {
        _outcome = outcome;
        _busy = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _busy = false);
      _toast(e.message);
    }
  }

  Future<void> _review() async {
    final item = _item;
    if (item == null || _busy) return;
    if (_code.trim().isEmpty) {
      _toast('Сначала напишите код');
      return;
    }
    setState(() {
      _busy = true;
      _verdict = null;
      _reviewError = null;
    });
    try {
      final verdict = await widget.api.review(questionId: item.id, code: _code);
      if (!mounted) return;
      setState(() {
        _verdict = verdict;
        _busy = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _reviewError = e.message;
      });
    }
  }

  Future<void> _enqueue() async {
    final item = _item;
    if (item == null) return;
    try {
      await widget.api.enqueue(item.id);
      _toast('В очереди на Web');
    } on ApiException catch (e) {
      _toast(e.message);
    }
  }

  Future<void> _copyLink() async {
    final item = _item;
    if (item == null) return;
    final origin = Uri.base.origin;
    await Clipboard.setData(ClipboardData(text: '$origin/?item=${item.id}'));
    _toast('Ссылка скопирована');
  }

  void _toast(String text) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(
        child: CircularProgressIndicator(color: AppColors.accent),
      );
    }
    if (_error != null && _item == null) {
      return _Message(text: _error!, action: 'Повторить', onPressed: _load);
    }
    final item = _item;
    if (item == null) {
      return _Message(
        text: _empty ?? 'Карточек нет',
        action: 'К планке',
        onPressed: widget.session.showHome,
      );
    }
    final wide = isWide(context);
    if (item.isCode && wide) return _codeSplit(item);
    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: ListView(
          padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, 28),
          children: [
            if (item.isCode)
              _mobileCode(item)
            else if (item.itemType == 'quiz')
              _quiz(item)
            else
              _card(item),
          ],
        ),
      ),
    );
  }

  Widget _quiz(PrepItem item) {
    final correct = _outcome?.correctIndex ?? item.correctIndex;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _Head(item: item),
        const SizedBox(height: 16),
        if (item.options.isEmpty)
          const Text(
            'У квиза нет вариантов ответа.',
            style: TextStyle(color: AppColors.muted),
          )
        else
          for (var i = 0; i < item.options.length; i++)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: _OptionButton(
                index: i,
                text: item.options[i],
                selected: _selected == i,
                correct: _outcome != null && correct == i,
                wrong: _outcome != null && _selected == i && correct != i,
                onPressed: _outcome == null && !_busy
                    ? () => _answerQuiz(i)
                    : null,
              ),
            ),
        if (_outcome != null) ...[
          const SizedBox(height: 8),
          Text(
            _outcome!.passed ? 'Верно' : 'Мимо',
            style: TextStyle(
              color: _outcome!.passed ? AppColors.ok : AppColors.bad,
              fontWeight: FontWeight.w700,
            ),
          ),
          if ((_outcome!.explanation.isNotEmpty
                  ? _outcome!.explanation
                  : item.explanation)
              .isNotEmpty) ...[
            const SizedBox(height: 8),
            Panel(
              child: SelectableText(
                _outcome!.explanation.isNotEmpty
                    ? _outcome!.explanation
                    : item.explanation,
                style: const TextStyle(height: 1.45),
              ),
            ),
          ],
          const SizedBox(height: 12),
          FilledButton(onPressed: () => _next(), child: const Text('Дальше')),
        ],
      ],
    );
  }

  Widget _card(PrepItem item) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _Head(item: item),
        const SizedBox(height: 16),
        if (!_revealed)
          OutlinedButton(
            onPressed: () => setState(() => _revealed = true),
            child: const Text('Показать скелет ответа'),
          )
        else ...[
          Panel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SelectableText(
                  item.answerSkeleton.isEmpty
                      ? 'Эталон в карточке пуст.'
                      : item.answerSkeleton,
                  style: const TextStyle(height: 1.45),
                ),
                if (item.checklist.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  for (final line in item.checklist)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 4),
                      child: Text(
                        '· $line',
                        style: const TextStyle(color: AppColors.muted),
                      ),
                    ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 12),
          if (_outcome == null)
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                FilledButton(
                  onPressed: _busy ? null : () => _answerCard(true),
                  child: const Text('Ответил четко ✅'),
                ),
                OutlinedButton(
                  onPressed: _busy ? null : () => _answerCard(false),
                  child: const Text('Поплыл (в Hard Concepts) ❌'),
                ),
              ],
            )
          else ...[
            Text(
              _outcome!.passed ? 'Зачтено себе' : 'Ушло в hard concepts',
              style: TextStyle(
                color: _outcome!.passed ? AppColors.ok : AppColors.warn,
                fontWeight: FontWeight.w700,
              ),
            ),
            const SizedBox(height: 12),
            FilledButton(onPressed: () => _next(), child: const Text('Дальше')),
          ],
        ],
      ],
    );
  }

  Widget _mobileCode(PrepItem item) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const MetaChip(
          text: '💻 Требует кода — открой в Web',
          color: AppColors.warn,
        ),
        const SizedBox(height: 12),
        _Head(item: item),
        const SizedBox(height: 12),
        SizedBox(
          height: 260,
          child: PrepCodeEditor(
            key: ValueKey('ro-${item.id}'),
            initialText: item.starterCode,
            language: item.language,
            readOnly: true,
            expands: true,
          ),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            FilledButton(
              onPressed: () => _next(oral: true),
              child: const Text('Перейти к следующему устному вопросу'),
            ),
            OutlinedButton(
              onPressed: _enqueue,
              child: const Text('Отложить в Web-очередь'),
            ),
            OutlinedButton(
              onPressed: _copyLink,
              child: const Text('Скопировать ссылку для ПК'),
            ),
          ],
        ),
      ],
    );
  }

  Widget _codeSplit(PrepItem item) {
    return CallbackShortcuts(
      bindings: {
        const SingleActivator(LogicalKeyboardKey.enter, control: true): _review,
        const SingleActivator(LogicalKeyboardKey.enter, meta: true): _review,
      },
      child: Focus(
        autofocus: true,
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(
                flex: 5,
                child: ListView(
                  children: [
                    _Head(item: item),
                    if (item.checklist.isNotEmpty) ...[
                      const SizedBox(height: 16),
                      const Text(
                        'Чек-лист',
                        style: TextStyle(fontWeight: FontWeight.w600),
                      ),
                      const SizedBox(height: 8),
                      for (final line in item.checklist)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 6),
                          child: Text('· $line'),
                        ),
                    ],
                    if (item.reviewCriteria.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      Panel(
                        child: SelectableText(
                          item.reviewCriteria,
                          style: const TextStyle(height: 1.45),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 16),
              Expanded(
                flex: 6,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Expanded(
                      child: PrepCodeEditor(
                        key: ValueKey('edit-${item.id}'),
                        initialText: item.starterCode,
                        language: item.language,
                        expands: true,
                        onChanged: (value) => _code = value,
                      ),
                    ),
                    const SizedBox(height: 10),
                    if (_reviewError != null) ...[
                      Text(
                        _reviewError!,
                        style: const TextStyle(color: AppColors.bad),
                      ),
                      const SizedBox(height: 8),
                    ],
                    Row(
                      children: [
                        FilledButton(
                          onPressed: _busy ? null : _review,
                          child: Text(
                            _busy
                                ? 'Смотрит старший…'
                                : 'На ревью (Ctrl+Enter)',
                          ),
                        ),
                        const SizedBox(width: 8),
                        OutlinedButton(
                          onPressed: _busy ? null : () => _next(),
                          child: const Text('Следующая'),
                        ),
                      ],
                    ),
                    if (_verdict != null) ...[
                      const SizedBox(height: 10),
                      _Verdict(verdict: _verdict!),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Head extends StatelessWidget {
  const _Head({required this.item});

  final PrepItem item;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: [
            if (item.blockTitle.isNotEmpty) MetaChip(text: item.blockTitle),
            if (item.company.isNotEmpty)
              MetaChip(text: item.company, color: AppColors.warn),
            MetaChip(text: item.itemType, color: AppColors.muted),
          ],
        ),
        const SizedBox(height: 12),
        SelectableText(
          item.heading,
          style: const TextStyle(
            fontSize: 22,
            fontWeight: FontWeight.w700,
            height: 1.25,
            letterSpacing: -0.4,
          ),
        ),
        if (item.title.trim().isNotEmpty &&
            item.prompt.trim().isNotEmpty &&
            item.prompt.trim() != item.title.trim()) ...[
          const SizedBox(height: 10),
          SelectableText(
            item.prompt,
            style: const TextStyle(fontSize: 16, height: 1.45),
          ),
        ],
      ],
    );
  }
}

class _OptionButton extends StatelessWidget {
  const _OptionButton({
    required this.index,
    required this.text,
    required this.selected,
    required this.correct,
    required this.wrong,
    required this.onPressed,
  });

  final int index;
  final String text;
  final bool selected;
  final bool correct;
  final bool wrong;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    final color = correct
        ? AppColors.ok
        : wrong
        ? AppColors.bad
        : AppColors.line;
    return Material(
      color: AppColors.surface,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onPressed,
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: color,
              width: correct || wrong || selected ? 1.4 : 1,
            ),
          ),
          child: Row(
            children: [
              Text(
                String.fromCharCode(65 + index),
                style: const TextStyle(
                  color: AppColors.muted,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(child: Text(text, style: const TextStyle(fontSize: 16))),
            ],
          ),
        ),
      ),
    );
  }
}

class _Verdict extends StatelessWidget {
  const _Verdict({required this.verdict});

  final ReviewVerdict verdict;

  @override
  Widget build(BuildContext context) {
    final color = verdict.passed ? AppColors.ok : AppColors.bad;
    return Container(
      constraints: const BoxConstraints(maxHeight: 220),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: color.withValues(alpha: 0.7)),
      ),
      child: ListView(
        shrinkWrap: true,
        children: [
          Text(
            verdict.passed
                ? 'Зачтено · ${verdict.score}/5'
                : 'Не зачтено · ${verdict.score}/5',
            style: TextStyle(color: color, fontWeight: FontWeight.w700),
          ),
          if (verdict.bugs.isNotEmpty) ...[
            const SizedBox(height: 8),
            const Text('Баги', style: TextStyle(fontWeight: FontWeight.w600)),
            for (final bug in verdict.bugs) Text('· $bug'),
          ],
          if (verdict.styleNotes.isNotEmpty) ...[
            const SizedBox(height: 8),
            const Text(
              'Заметки',
              style: TextStyle(fontWeight: FontWeight.w600),
            ),
            for (final note in verdict.styleNotes) Text('· $note'),
          ],
          if (verdict.followUp.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text('Вдогонку: ${verdict.followUp}'),
          ],
        ],
      ),
    );
  }
}

class _Message extends StatelessWidget {
  const _Message({
    required this.text,
    required this.action,
    required this.onPressed,
  });

  final String text;
  final String action;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 460),
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                text,
                textAlign: TextAlign.center,
                style: const TextStyle(color: AppColors.muted, height: 1.4),
              ),
              const SizedBox(height: 16),
              OutlinedButton(onPressed: onPressed, child: Text(action)),
            ],
          ),
        ),
      ),
    );
  }
}
