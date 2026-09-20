package usersettings

// TimetableProfileVisibilityKey stores whether a user's timetable is shown on
// their public profile ("true"/"false" via ManageUserSettingUsecase). Unset means
// visible — this preserves the default of the old timetables.is_profile_visible
// column that this per-user setting replaced.
const TimetableProfileVisibilityKey = "timetableProfileVisibility"
