import 'package:flutter/material.dart';
import 'package:flutter_code_editor/flutter_code_editor.dart';
import 'package:flutter_highlight/themes/monokai-sublime.dart';
import 'package:highlight/languages/go.dart';
import 'package:highlight/languages/sql.dart';

import '../theme.dart';

class PrepCodeEditor extends StatefulWidget {
  const PrepCodeEditor({
    super.key,
    required this.initialText,
    required this.language,
    this.readOnly = false,
    this.expands = false,
    this.onChanged,
  });

  final String initialText;
  final String language;
  final bool readOnly;
  final bool expands;
  final ValueChanged<String>? onChanged;

  @override
  State<PrepCodeEditor> createState() => _PrepCodeEditorState();
}

class _PrepCodeEditorState extends State<PrepCodeEditor> {
  late final CodeController _controller;

  @override
  void initState() {
    super.initState();
    _controller = CodeController(
      text: widget.initialText,
      language: widget.language.toLowerCase() == 'sql' ? sql : go,
      readOnly: widget.readOnly,
    );
    _controller.addListener(() => widget.onChanged?.call(_controller.text));
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: const Color(0xFF12141A),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.line),
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(12),
        child: CodeTheme(
          data: CodeThemeData(styles: monokaiSublimeTheme),
          child: CodeField(
            controller: _controller,
            expands: widget.expands,
            readOnly: widget.readOnly,
            wrap: false,
            textStyle: const TextStyle(
              fontFamily: 'monospace',
              fontSize: 13.5,
              height: 1.45,
              color: AppColors.text,
            ),
            gutterStyle: const GutterStyle(
              width: 48,
              margin: 8,
              showErrors: false,
              showFoldingHandles: false,
              textStyle: TextStyle(color: AppColors.muted),
              background: Color(0xFF12141A),
            ),
            background: const Color(0xFF12141A),
          ),
        ),
      ),
    );
  }
}
