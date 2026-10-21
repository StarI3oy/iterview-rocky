import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../api.dart';
import '../models.dart';
import '../session.dart';
import '../theme.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({
    super.key,
    required this.api,
    required this.session,
    this.pickFile,
  });

  final Api api;
  final Session session;

  /// When set, tests supply a file without opening the system dialog.
  final Future<({Uint8List bytes, String name})?> Function()? pickFile;

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  BarStatus? _bar;
  List<PrepItem> _queue = const [];
  List<HardConcept> _hard = const [];
  String? _error;
  bool _loading = true;
  bool _importing = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        widget.api.bar(),
        widget.api.queue(),
        widget.api.hard(),
      ]);
      if (!mounted) return;
      setState(() {
        _bar = results[0] as BarStatus;
        _queue = results[1] as List<PrepItem>;
        _hard = results[2] as List<HardConcept>;
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

  Future<void> _importFile() async {
    final picked = widget.pickFile != null
        ? await widget.pickFile!()
        : await _pickFromDisk();
    if (picked == null || !mounted) return;
    final bytes = picked.bytes;
    final name = picked.name;
    setState(() => _importing = true);
    try {
      final stats = await widget.api.importFile(bytes, name);
      if (!mounted) return;
      await _load();
      if (!mounted) return;
      final extra = [
        if (stats.questions > 0) 'вопросов ${stats.questions}',
        if (stats.prepItems > 0) 'карточек подготовки ${stats.prepItems}',
        if (stats.concepts > 0) 'концептов ${stats.concepts}',
      ].join(', ');
      _toast(
        extra.isEmpty
            ? 'В базу записано карточек: ${stats.cards}'
            : 'В базу записано карточек: ${stats.cards} ($extra)',
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      _toast(e.message);
    } finally {
      if (mounted) setState(() => _importing = false);
    }
  }

  Future<({Uint8List bytes, String name})?> _pickFromDisk() async {
    final picked = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: const ['json', 'sql', 'txt'],
      withData: true,
    );
    if (picked == null || picked.files.isEmpty) return null;
    final file = picked.files.single;
    final bytes = file.bytes;
    if (bytes == null) {
      _toast('Не удалось прочитать файл');
      return null;
    }
    return (bytes: bytes, name: file.name);
  }

  void _toast(String message) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWide(context);
    return RefreshIndicator(
      color: AppColors.accent,
      backgroundColor: AppColors.surface,
      onRefresh: _load,
      child: ListView(
        padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, 28),
        children: [
          const Text(
            'Планка 400к+',
            style: TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w700,
              letterSpacing: -0.6,
            ),
          ),
          const SizedBox(height: 6),
          const Text(
            'Go, Postgres/Kafka, алгоритмы, system design',
            style: TextStyle(color: AppColors.muted),
          ),
          const SizedBox(height: 18),
          if (_loading && _bar == null)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 48),
              child: Center(
                child: CircularProgressIndicator(color: AppColors.accent),
              ),
            )
          else if (_error != null && _bar == null)
            Panel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(_error!, style: const TextStyle(color: AppColors.bad)),
                  const SizedBox(height: 12),
                  OutlinedButton(
                    onPressed: _load,
                    child: const Text('Повторить'),
                  ),
                ],
              ),
            )
          else if (_bar != null) ...[
            _Summary(bar: _bar!),
            const SizedBox(height: 14),
            _ImportPanel(importing: _importing, onImport: _importFile),
            const SizedBox(height: 14),
            _Blocks(bar: _bar!),
            const SizedBox(height: 18),
            if (_bar!.protocol.isNotEmpty) ...[
              const Text(
                'Протокол',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              Panel(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxHeight: 160),
                  child: SingleChildScrollView(
                    child: SelectableText(
                      _bar!.protocol,
                      style: const TextStyle(height: 1.4),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 16),
            ],
            FilledButton(
              onPressed: widget.session.openSprint,
              child: Text(
                _bar!.today.isEmpty
                    ? 'Утренний спринт'
                    : 'Утренний спринт · ${_bar!.today}',
              ),
            ),
            const SizedBox(height: 10),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final block in _bar!.blocks)
                  OutlinedButton(
                    onPressed: () => widget.session.openTrack(block.id),
                    child: Text(block.title),
                  ),
                OutlinedButton(
                  onPressed: widget.session.openCode,
                  child: const Text('Только кодовые задачи (Web)'),
                ),
                OutlinedButton(
                  onPressed: () => widget.session.openTrack('concept'),
                  child: Text('Разобранные концепты (${_bar!.studied})'),
                ),
                if (_hard.isNotEmpty)
                  OutlinedButton(
                    onPressed: widget.session.openHard,
                    child: Text('Провалы (${_hard.length})'),
                  ),
              ],
            ),
            if (_bar!.tracker.isNotEmpty) ...[
              const SizedBox(height: 22),
              const Text(
                'Уже спрашивали',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              for (final item in _bar!.tracker)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Panel(
                    padding: const EdgeInsets.all(12),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          item.topic,
                          style: const TextStyle(
                            color: AppColors.muted,
                            fontSize: 12,
                          ),
                        ),
                        const SizedBox(height: 4),
                        Text(item.question),
                        const SizedBox(height: 4),
                        Text(
                          'раз ${item.timesAsked} · ${item.understood ? 'понял' : 'ещё плавает'}',
                          style: const TextStyle(
                            color: AppColors.muted,
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
            if (_queue.isNotEmpty) ...[
              const SizedBox(height: 22),
              const Text(
                'Очередь Web-задач',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              for (final item in _queue)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: _QueueTile(
                    item: item,
                    onTap: () => widget.session.openItem(item.id),
                  ),
                ),
            ],
          ],
        ],
      ),
    );
  }
}

class _ImportPanel extends StatelessWidget {
  const _ImportPanel({required this.importing, required this.onImport});

  final bool importing;
  final VoidCallback onImport;

  @override
  Widget build(BuildContext context) {
    return Panel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Пополнить базу',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 6),
          const Text(
            'JSON-экспорт, SQL-дамп (.sql) или текстовый дамп (.txt). Повторная загрузка обновляет карточки и не сбрасывает прогресс тренировок.',
            style: TextStyle(color: AppColors.muted, height: 1.4),
          ),
          const SizedBox(height: 12),
          FilledButton.icon(
            onPressed: importing ? null : onImport,
            icon: importing
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  )
                : const Icon(Icons.upload_file),
            label: Text(importing ? 'Загрузка…' : 'Загрузить файл'),
          ),
        ],
      ),
    );
  }
}

class _Summary extends StatelessWidget {
  const _Summary({required this.bar});

  final BarStatus bar;

  @override
  Widget build(BuildContext context) {
    return Panel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            '${bar.overall}%',
            style: const TextStyle(
              fontSize: 56,
              fontWeight: FontWeight.w700,
              height: 1,
              letterSpacing: -1.5,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            bar.empty
                ? 'База знаний ещё не загружена'
                : 'Закрыто ${bar.done} из ${bar.total}',
            style: const TextStyle(color: AppColors.muted),
          ),
          const SizedBox(height: 12),
          ClipRRect(
            borderRadius: BorderRadius.circular(99),
            child: LinearProgressIndicator(
              value: bar.total == 0 ? 0 : bar.done / bar.total,
              minHeight: 8,
              backgroundColor: AppColors.track,
              color: AppColors.accent,
            ),
          ),
        ],
      ),
    );
  }
}

class _Blocks extends StatelessWidget {
  const _Blocks({required this.bar});

  final BarStatus bar;

  @override
  Widget build(BuildContext context) {
    return Panel(
      child: Column(
        children: [
          for (var i = 0; i < bar.blocks.length; i++) ...[
            if (i > 0) const SizedBox(height: 14),
            _BlockRow(
              stat: bar.blocks[i],
              color: AppColors.blocks[i % AppColors.blocks.length],
            ),
          ],
        ],
      ),
    );
  }
}

class _BlockRow extends StatelessWidget {
  const _BlockRow({required this.stat, required this.color});

  final BlockStat stat;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                stat.title,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ),
            Text(
              stat.total == 0
                  ? '0%'
                  : '${stat.percent}% · ${stat.done}/${stat.total}',
              style: const TextStyle(color: AppColors.muted, fontSize: 12),
            ),
          ],
        ),
        const SizedBox(height: 6),
        ClipRRect(
          borderRadius: BorderRadius.circular(99),
          child: LinearProgressIndicator(
            value: stat.total == 0 ? 0 : stat.done / stat.total,
            minHeight: 6,
            backgroundColor: AppColors.track,
            color: color,
          ),
        ),
      ],
    );
  }
}

class _QueueTile extends StatelessWidget {
  const _QueueTile({required this.item, required this.onTap});

  final PrepItem item;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: AppColors.surface,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: AppColors.line),
          ),
          child: Row(
            children: [
              const Text('💻', style: TextStyle(fontSize: 16)),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  item.heading,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
