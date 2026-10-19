import 'package:flutter/foundation.dart';

class Session extends ChangeNotifier {
  int tab = 0;
  String mode = 'mobile';
  String kind = 'sprint';
  int? focusId;
  int epoch = 0;

  void showHome() {
    tab = 0;
    notifyListeners();
  }

  void showSearch() {
    tab = 2;
    notifyListeners();
  }

  void openSprint() => _open(mode: 'mobile', kind: 'sprint');

  void openCode() => _open(mode: 'web', kind: 'code');

  void openHard() => _open(mode: 'mobile', kind: 'hard');

  void openTrack(String track) {
    const known = {
      'dumps',
      'patterns',
      'system_design',
      'legend',
      'library',
      'concept',
    };
    if (!known.contains(track)) {
      openSprint();
      return;
    }
    final mode = track == 'patterns' ? 'web' : 'mobile';
    _open(mode: mode, kind: track);
  }

  void openItem(int id) => _open(mode: mode, kind: kind, focus: id);

  void _open({required String mode, required String kind, int? focus}) {
    this.mode = mode;
    this.kind = kind;
    focusId = focus;
    epoch++;
    tab = 1;
    notifyListeners();
  }
}
