import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../api.dart';
import '../models.dart';
import '../theme.dart';

class ImportScreen extends StatefulWidget {
  const ImportScreen({
    super.key,
    required this.api,
    this.pickFile,
    this.onChangePassword,
    this.onLogout,
  });

  final Api api;
  final Future<({Uint8List bytes, String name})?> Function()? pickFile;
  final VoidCallback? onChangePassword;
  final VoidCallback? onLogout;

  @override
  State<ImportScreen> createState() => _ImportScreenState();
}

class _ImportScreenState extends State<ImportScreen> {
  Uint8List? _bytes;
  String? _name;
  ImportStats? _preview;
  String? _note;
  bool _noteBad = false;
  bool _busy = false;

  Future<void> _choose() async {
    final picked = widget.pickFile != null
        ? await widget.pickFile!()
        : await _pickFromDisk();
    if (picked == null || !mounted) return;
    setState(() {
      _busy = true;
      _bytes = picked.bytes;
      _name = picked.name;
      _preview = null;
      _note = null;
      _noteBad = false;
    });
    try {
      final stats = await widget.api.previewFile(picked.bytes, picked.name);
      if (!mounted) return;
      setState(() {
        _preview = stats;
        _noteBad = stats.rejected;
        _note = stats.rejected
            ? 'Ошибка формата в краеугольном поле — весь список не проходит'
            : 'Можно записать ${stats.preview.length} записей';
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _note = e.message;
        _noteBad = true;
        _preview = null;
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _commit() async {
    final bytes = _bytes;
    final name = _name;
    final preview = _preview;
    if (bytes == null || name == null || preview == null || preview.rejected) {
      return;
    }
    setState(() => _busy = true);
    try {
      final stats = await widget.api.importFile(bytes, name);
      if (!mounted) return;
      setState(() {
        _preview = stats;
        _noteBad = stats.rejected || stats.cards == 0;
        _note = stats.rejected
            ? 'Ошибка формата в краеугольном поле — весь список не проходит'
            : 'В базу записано карточек: ${stats.cards}';
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _note = e.message;
        _noteBad = true;
      });
    } finally {
      if (mounted) setState(() => _busy = false);
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
    if (bytes == null) return null;
    return (bytes: bytes, name: file.name);
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWide(context);
    final preview = _preview;
    final canCommit =
        !_busy && preview != null && !preview.rejected && _bytes != null;
    return ListView(
      padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, 28),
      children: [
        const Text(
          'Импорт',
          style: TextStyle(
            fontSize: 28,
            fontWeight: FontWeight.w700,
            letterSpacing: -0.6,
          ),
        ),
        const SizedBox(height: 8),
        const Text(
          'JSON, SQL-дамп или текстовый дамп. Краеугольные поля — id, тема, категория, вопрос, ответ, грейд, сложность и дата. Ошибка формата в любом из них отклоняет весь список, в базу ничего не пишется.',
          style: TextStyle(color: AppColors.muted, height: 1.4),
        ),
        const SizedBox(height: 16),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            FilledButton.icon(
              onPressed: _busy ? null : _choose,
              icon: _busy
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: Colors.white,
                      ),
                    )
                  : const Icon(Icons.upload_file),
              label: Text(_busy ? 'Проверка…' : 'Выбрать файл'),
            ),
            FilledButton(
              onPressed: canCommit ? _commit : null,
              child: const Text('Записать в базу'),
            ),
          ],
        ),
        if (_name != null) ...[
          const SizedBox(height: 10),
          Text(_name!, style: const TextStyle(color: AppColors.muted)),
        ],
        if (_note != null) ...[
          const SizedBox(height: 12),
          Text(
            _note!,
            style: TextStyle(
              color: _noteBad ? AppColors.bad : AppColors.ok,
              fontWeight: FontWeight.w600,
              height: 1.4,
            ),
          ),
        ],
        if (preview != null && preview.issues.isNotEmpty) ...[
          const SizedBox(height: 12),
          for (final issue in preview.issues.take(40))
            Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Text(
                issue.label,
                style: const TextStyle(color: AppColors.bad, height: 1.35),
              ),
            ),
          if (preview.issues.length > 40)
            Text(
              'и ещё ${preview.issues.length - 40}',
              style: const TextStyle(color: AppColors.bad),
            ),
        ],
        if (preview != null && preview.preview.isNotEmpty) ...[
          const SizedBox(height: 18),
          const Text(
            'Потенциальный список',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          for (final row in preview.preview.take(200))
            Padding(
              padding: const EdgeInsets.only(bottom: 6),
              child: Text(
                row.label,
                style: TextStyle(
                  color: row.ok ? AppColors.text : AppColors.bad,
                  height: 1.35,
                ),
              ),
            ),
          if (preview.preview.length > 200)
            Text(
              'и ещё ${preview.preview.length - 200}',
              style: const TextStyle(color: AppColors.muted),
            ),
        ],
        if (!wide &&
            (widget.onChangePassword != null || widget.onLogout != null)) ...[
          const SizedBox(height: 28),
          const Text('Доступ', style: TextStyle(fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          if (widget.onChangePassword != null)
            OutlinedButton(
              onPressed: widget.onChangePassword,
              child: const Text('Сменить пароль'),
            ),
          if (widget.onLogout != null) ...[
            const SizedBox(height: 8),
            OutlinedButton(
              onPressed: widget.onLogout,
              child: const Text('Выйти'),
            ),
          ],
        ],
      ],
    );
  }
}
